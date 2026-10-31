package app

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"com-itemgather/internal/security"
)

//go:embed ai_prompt_v1.txt
var aiSystemPromptV1 string

const aiPromptVersion = "v1"

type AIResult struct {
	DisplayTitle      string      `json:"display_title"`
	Summary           string      `json:"summary"`
	NameCandidates    []string    `json:"name_candidates"`
	IdeaTypes         []string    `json:"idea_types"`
	ProductDirections []string    `json:"product_directions"`
	Tags              []string    `json:"tags"`
	Attributes        []Attribute `json:"attributes"`
}
type AITask struct {
	ID, IdeaID, SourceContent, Mode, Status, LastError         string
	SourceContentVersion, SourceVersion, Attempts, MaxAttempts int
}
type aiCallError struct {
	message    string
	retryable  bool
	retryAfter time.Duration
}

func (e *aiCallError) Error() string { return e.message }

func parseAIResult(raw string) (AIResult, error) {
	var result AIResult
	if strings.TrimSpace(raw) == "" {
		return result, errors.New("模型返回了空白响应")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return result, fmt.Errorf("模型返回了非法 JSON: %w", err)
	}
	if decoder.More() {
		return result, errors.New("模型响应包含额外内容")
	}
	required := []string{"display_title", "summary", "name_candidates", "idea_types", "product_directions", "tags", "attributes"}
	if len(fields) != len(required) {
		return result, errors.New("模型响应字段数量不符合约定")
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok {
			return result, fmt.Errorf("模型响应缺少字段 %s", key)
		}
		var target any
		switch key {
		case "display_title":
			target = &result.DisplayTitle
		case "summary":
			target = &result.Summary
		case "name_candidates":
			target = &result.NameCandidates
		case "idea_types":
			target = &result.IdeaTypes
		case "product_directions":
			target = &result.ProductDirections
		case "tags":
			target = &result.Tags
		case "attributes":
			target = &result.Attributes
		}
		fieldDecoder := json.NewDecoder(bytes.NewReader(value))
		fieldDecoder.DisallowUnknownFields()
		if err := fieldDecoder.Decode(target); err != nil {
			return result, fmt.Errorf("字段 %s 类型或结构不正确: %w", key, err)
		}
		if string(value) == "null" {
			return result, fmt.Errorf("字段 %s 不能为 null", key)
		}
	}
	if len([]rune(result.DisplayTitle)) > 160 || len([]rune(result.Summary)) > 4000 {
		return result, errors.New("标题或摘要超出长度限制")
	}
	if len(result.NameCandidates) > 100 || len(result.IdeaTypes) > 30 || len(result.ProductDirections) > 30 || len(result.Tags) > 50 {
		return result, errors.New("模型返回的数组过长")
	}
	result.NameCandidates = validStringList(result.NameCandidates, 100)
	result.IdeaTypes = validStringList(result.IdeaTypes, 30)
	result.ProductDirections = validStringList(result.ProductDirections, 30)
	result.Tags = validStringList(result.Tags, 50)
	for index, attr := range result.Attributes {
		if strings.TrimSpace(attr.Key) == "" || len([]rune(attr.Key)) > 80 || len([]rune(attr.Value)) > 2000 {
			return result, fmt.Errorf("属性 %d 的键值无效", index)
		}
		if attr.Certainty != "asserted" && attr.Certainty != "tentative" && attr.Certainty != "undecided" {
			return result, fmt.Errorf("属性 %d 的 certainty 无效", index)
		}
	}
	attrs, err := validAttributes(result.Attributes)
	if err != nil {
		return result, err
	}
	result.Attributes = attrs
	return result, nil
}

func (a *App) organize(ctx context.Context, content string) (AIResult, error) {
	var result AIResult
	if a.cfg.AIAPIKey == "" {
		return result, &aiCallError{message: "尚未配置 AI API Key", retryable: false}
	}
	requestBody := map[string]any{"model": a.cfg.AIModel, "messages": []map[string]string{{"role": "system", "content": aiSystemPromptV1}, {"role": "user", "content": content}}, "response_format": map[string]string{"type": "json_object"}, "temperature": 0}
	encoded, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.AIBaseURL+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return result, &aiCallError{message: "AI 请求配置无效", retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.AIAPIKey)
	client := http.Client{Timeout: a.cfg.AITimeout}
	response, err := client.Do(req)
	if err != nil {
		return result, &aiCallError{message: "AI 请求失败或超时: " + clean(err.Error(), 200), retryable: true}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return result, &aiCallError{message: "无法读取 AI 响应", retryable: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retryable := response.StatusCode == 408 || response.StatusCode == 409 || response.StatusCode == 429 || response.StatusCode >= 500
		callErr := &aiCallError{message: fmt.Sprintf("AI 服务返回 HTTP %d", response.StatusCode), retryable: retryable}
		if response.StatusCode == 429 {
			callErr.retryAfter = parseRetryAfter(response.Header.Get("Retry-After"))
		}
		return result, callErr
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return result, &aiCallError{message: "AI 接口响应不是合法 JSON", retryable: true}
	}
	if payload.Error != nil {
		return result, &aiCallError{message: "AI 接口返回错误", retryable: true}
	}
	if len(payload.Choices) == 0 {
		return result, &aiCallError{message: "AI 接口没有返回整理结果", retryable: true}
	}
	if payload.Choices[0].FinishReason == "length" {
		return result, &aiCallError{message: "AI 输出被截断", retryable: true}
	}
	result, err = parseAIResult(payload.Choices[0].Message.Content)
	if err != nil {
		return result, &aiCallError{message: err.Error(), retryable: true}
	}
	return result, nil
}
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		if delay := time.Until(date); delay > 0 {
			return delay
		}
	}
	return 0
}

type AIWorker struct {
	a    *App
	wake chan struct{}
}

func NewAIWorker(a *App) *AIWorker { return &AIWorker{a: a, wake: make(chan struct{}, 1)} }
func (w *AIWorker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
func (w *AIWorker) Start(ctx context.Context) {
	w.a.db.ExecContext(context.Background(), "UPDATE ai_tasks SET status='pending',leased_until=NULL,updated_at=? WHERE status='running'", now())
	w.a.db.ExecContext(context.Background(), "UPDATE ideas SET ai_status='pending' WHERE ai_status='running'")
	for i := 0; i < w.a.cfg.AIWorkers; i++ {
		go w.run(ctx)
	}
}
func (w *AIWorker) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
		for {
			task, ok, err := w.claim(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.a.log.Printf("AI task claim failed: %v", err)
				}
				break
			}
			if !ok {
				break
			}
			w.process(ctx, task)
		}
	}
}
func (w *AIWorker) claim(ctx context.Context) (AITask, bool, error) {
	var task AITask
	lease := time.Now().UTC().Add(w.a.cfg.AITimeout + 30*time.Second).Format(time.RFC3339Nano)
	err := w.a.db.QueryRowContext(ctx, `UPDATE ai_tasks SET status='running',leased_until=?,updated_at=? WHERE id=(SELECT id FROM ai_tasks WHERE status='pending' AND next_attempt_at<=? ORDER BY created_at LIMIT 1) AND status='pending' RETURNING id,idea_id,source_content,source_content_version,source_version,mode,status,attempts,max_attempts,COALESCE(last_error,'')`, lease, now(), now()).Scan(&task.ID, &task.IdeaID, &task.SourceContent, &task.SourceContentVersion, &task.SourceVersion, &task.Mode, &task.Status, &task.Attempts, &task.MaxAttempts, &task.LastError)
	if err == sql.ErrNoRows {
		return task, false, nil
	}
	if err != nil {
		return task, false, err
	}
	w.a.db.ExecContext(ctx, "UPDATE ideas SET ai_status='running',ai_error='',updated_at=? WHERE id=? AND content_version=?", now(), task.IdeaID, task.SourceContentVersion)
	w.a.emit("idea.ai_changed", "idea", task.IdeaID)
	return task, true, nil
}
func (a *App) enqueueAITx(ctx context.Context, tx *sql.Tx, ideaID, content string, contentVersion, version int, mode string) error {
	id := security.ID()
	timestamp := now()
	_, err := tx.ExecContext(ctx, "INSERT INTO ai_tasks(id,idea_id,source_content,source_content_version,source_version,mode,status,max_attempts,next_attempt_at,created_at,updated_at) VALUES(?,?,?,?,?,?,'pending',?,?,?,?)", id, ideaID, content, contentVersion, version, mode, a.cfg.AIMaxRetries+1, timestamp, timestamp, timestamp)
	return err
}

func (w *AIWorker) process(ctx context.Context, task AITask) {
	result, err := w.a.organize(ctx, task.SourceContent)
	attempt := task.Attempts + 1
	if err != nil {
		var callErr *aiCallError
		retryable := errors.As(err, &callErr) && callErr.retryable
		if !errors.As(err, &callErr) {
			callErr = &aiCallError{message: err.Error(), retryable: true}
			retryable = true
		}
		finished := !retryable || attempt >= task.MaxAttempts
		delay := time.Duration(1<<(attempt-1)) * time.Second
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		if callErr.retryAfter > delay {
			delay = callErr.retryAfter
		}
		status := "pending"
		ideaStatus := "pending"
		if finished {
			status = "failed"
			ideaStatus = "failed"
		}
		next := time.Now().UTC().Add(delay).Format(time.RFC3339Nano)
		tx, txErr := w.a.db.BeginTx(context.Background(), nil)
		if txErr != nil {
			w.a.log.Printf("AI failure persistence failed: %v", txErr)
			return
		}
		defer tx.Rollback()
		var currentStatus string
		tx.QueryRow("SELECT status FROM ai_tasks WHERE id=?", task.ID).Scan(&currentStatus)
		if currentStatus != "running" {
			return
		}
		var contentVersion int
		tx.QueryRow("SELECT content_version FROM ideas WHERE id=?", task.IdeaID).Scan(&contentVersion)
		if contentVersion != task.SourceContentVersion {
			tx.Exec("UPDATE ai_tasks SET status='cancelled',attempts=?,last_error='',updated_at=? WHERE id=?", attempt, now(), task.ID)
			tx.Exec("UPDATE ideas SET ai_status='idle',ai_error='',updated_at=? WHERE id=? AND content_version=?", now(), task.IdeaID, task.SourceContentVersion)
		} else {
			visibleError := ""
			if finished {
				visibleError = callErr.message
			}
			tx.Exec("UPDATE ai_tasks SET status=?,attempts=?,next_attempt_at=?,leased_until=NULL,last_error=?,updated_at=? WHERE id=?", status, attempt, next, clean(callErr.message, 500), now(), task.ID)
			tx.Exec("UPDATE ideas SET ai_status=?,ai_error=?,updated_at=? WHERE id=?", ideaStatus, visibleError, now(), task.IdeaID)
		}
		if tx.Commit() == nil {
			w.a.emit("idea.ai_changed", "idea", task.IdeaID)
		}
		return
	}
	tx, err := w.a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var taskStatus string
	tx.QueryRow("SELECT status FROM ai_tasks WHERE id=?", task.ID).Scan(&taskStatus)
	if taskStatus != "running" {
		return
	}
	item, err := loadIdea(context.Background(), tx, task.IdeaID, "", true, false)
	if err != nil {
		return
	}
	if item.ContentVersion != task.SourceContentVersion {
		tx.Exec("UPDATE ai_tasks SET status='cancelled',attempts=?,updated_at=? WHERE id=?", attempt, now(), task.ID)
		if tx.Commit() == nil {
			w.a.emit("idea.ai_changed", "idea", task.IdeaID)
		}
		return
	}
	before := ideaSnapshot(item)
	manual := map[string]bool{}
	for _, field := range item.ManualFields {
		manual[field] = true
	}
	if task.Mode == "full" && task.SourceVersion > 0 {
		changed, err := organizedFieldsChangedSince(tx, task.IdeaID, task.SourceVersion)
		if err != nil {
			w.a.log.Printf("AI field protection lookup failed: %v", err)
			return
		}
		for field := range changedOrganizedFields(before, before) {
			if !changed[field] {
				delete(manual, field)
			}
		}
	}
	if manual["display_title"] {
		result.DisplayTitle = item.DisplayTitle
	}
	if manual["summary"] {
		result.Summary = item.Summary
	}
	if manual["name_candidates"] {
		result.NameCandidates = item.NameCandidates
	}
	if manual["idea_types"] {
		result.IdeaTypes = item.IdeaTypes
	}
	if manual["product_directions"] {
		result.ProductDirections = item.ProductDirections
	}
	if manual["tags"] {
		result.Tags = item.Tags
	}
	if manual["attributes"] {
		result.Attributes = item.Attributes
	}
	manualList := make([]string, 0, len(manual))
	for field := range manual {
		manualList = append(manualList, field)
	}
	slices.Sort(manualList)
	item.DisplayTitle = result.DisplayTitle
	item.Summary = result.Summary
	item.NameCandidates = result.NameCandidates
	item.IdeaTypes = result.IdeaTypes
	item.ProductDirections = result.ProductDirections
	item.Tags = result.Tags
	item.Attributes = result.Attributes
	item.ManualFields = manualList
	item.AIStatus = "succeeded"
	item.AIError = ""
	item.Version++
	item.UpdatedAt = now()
	_, err = tx.Exec(`UPDATE ideas SET display_title=?,summary=?,name_candidates_json=?,idea_types_json=?,product_directions_json=?,tags_json=?,manual_fields_json=?,ai_status='succeeded',ai_error='',version=?,updated_at=? WHERE id=?`, item.DisplayTitle, item.Summary, encodeJSON(item.NameCandidates), encodeJSON(item.IdeaTypes), encodeJSON(item.ProductDirections), encodeJSON(item.Tags), encodeJSON(manualList), item.Version, item.UpdatedAt, item.ID)
	if err != nil {
		return
	}
	if !manual["attributes"] {
		if err = replaceIdeaAttributes(context.Background(), tx, item.ID, item.Attributes); err != nil {
			return
		}
	}
	if !manual["tags"] {
		if err = replaceIdeaTags(context.Background(), tx, item.ID, item.Tags); err != nil {
			return
		}
	}
	if err = addRevision(context.Background(), tx, "idea", item.ID, item.Version, nil, "ai_organize_"+aiPromptVersion, before, ideaSnapshot(item)); err != nil {
		return
	}
	tx.Exec("UPDATE ai_tasks SET status='succeeded',attempts=?,leased_until=NULL,last_error='',updated_at=? WHERE id=?", attempt, now(), task.ID)
	if err = aPublish(tx, "idea.ai_succeeded", "idea", item.ID); err != nil {
		return
	}
	if err = tx.Commit(); err != nil {
		return
	}
	w.a.emit("idea.ai_changed", "idea", task.IdeaID)
}

func organizedFieldsChangedSince(tx *sql.Tx, ideaID string, version int) (map[string]bool, error) {
	rows, err := tx.Query(`SELECT before_data,after_data FROM revisions WHERE entity_type='idea' AND entity_id=? AND version>? AND action NOT LIKE 'ai_%'`, ideaID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changed := map[string]bool{}
	for rows.Next() {
		var beforeJSON, afterJSON string
		if err := rows.Scan(&beforeJSON, &afterJSON); err != nil {
			return nil, err
		}
		var before, after ideaFields
		if err := json.Unmarshal([]byte(beforeJSON), &before); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(afterJSON), &after); err != nil {
			return nil, err
		}
		for field, modified := range changedOrganizedFields(before, after) {
			if modified {
				changed[field] = true
			}
		}
	}
	return changed, rows.Err()
}
func aPublish(tx *sql.Tx, eventType, entityType, id string) error {
	_, err := tx.Exec("INSERT INTO events(event_type,entity_type,entity_id,created_at) VALUES(?,?,?,?)", eventType, entityType, id, now())
	return err
}
