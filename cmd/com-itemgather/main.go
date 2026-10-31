package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"com-itemgather/internal/app"
	"com-itemgather/internal/backup"
	"com-itemgather/internal/config"
	"com-itemgather/internal/database"
	"com-itemgather/internal/filelock"
	"com-itemgather/internal/security"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		usage()
		return fmt.Errorf("请指定命令")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	command := os.Args[1]
	switch command {
	case "serve":
		return serve(cfg)
	case "migrate":
		return withDB(cfg, func(db *sql.DB) error { return database.Migrate(context.Background(), db) })
	case "create-admin":
		return createAdmin(cfg, os.Args[2:])
	case "reset-password":
		return resetPassword(cfg, os.Args[2:])
	case "backup":
		return withDB(cfg, func(db *sql.DB) error {
			if err := database.Migrate(context.Background(), db); err != nil {
				return err
			}
			path, err := backup.New(db, cfg.AttachmentDir, cfg.BackupDir, cfg.BackupKeep, cfg.BackupRetentionDays).Create(context.Background())
			if err == nil {
				fmt.Println(path)
			}
			return err
		})
	case "restore":
		return restoreBackup(cfg, os.Args[2:])
	default:
		usage()
		return fmt.Errorf("未知命令 %s", command)
	}
}
func usage() {
	fmt.Println(`COM-ItemGather
用法：
  com-itemgather serve
  com-itemgather migrate
  com-itemgather create-admin -username NAME -nickname NAME [-password PASSWORD]
  com-itemgather reset-password -username NAME [-password PASSWORD]
  com-itemgather backup
  com-itemgather restore -archive BACKUP.tar.gz -yes
恢复前必须停止服务。恢复会保留被替换的数据；密码也可通过 ADMIN_PASSWORD 或 RESET_PASSWORD 环境变量传入。`)
}
func withDB(cfg config.Config, action func(*sql.DB) error) error {
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	return action(db)
}
func serve(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	unlock, err := filelock.TryAcquire(ctx, cfg.DatabasePath+".running.lock")
	if err != nil {
		return fmt.Errorf("database is in use by another server or restore: %w", err)
	}
	defer unlock()
	return withDB(cfg, func(db *sql.DB) error {
		if err := database.Migrate(ctx, db); err != nil {
			return err
		}
		logger := log.New(os.Stdout, "com-itemgather ", log.LstdFlags)
		application := app.New(db, cfg, logger)
		if err := application.Start(ctx); err != nil {
			return fmt.Errorf("recover AI tasks: %w", err)
		}
		manager := backup.New(db, cfg.AttachmentDir, cfg.BackupDir, cfg.BackupKeep, cfg.BackupRetentionDays)
		go func() {
			ticker := time.NewTicker(cfg.BackupInterval)
			defer ticker.Stop()
			for {
				if _, err := manager.Create(ctx); err != nil {
					if ctx.Err() == nil {
						logger.Printf("自动备份失败: %v", err)
					}
				} else if ctx.Err() == nil {
					logger.Printf("数据库与附件备份完成")
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		server := &http.Server{Addr: cfg.Addr, Handler: application, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
		errorChannel := make(chan error, 1)
		go func() { logger.Printf("监听 %s", cfg.Addr); errorChannel <- server.ListenAndServe() }()
		select {
		case <-ctx.Done():
		case err := <-errorChannel:
			if err != http.ErrServerClosed {
				return err
			}
		}
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	})
}
func restoreBackup(cfg config.Config, args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	archive := flags.String("archive", "", "备份 tar.gz 文件")
	confirmed := flags.Bool("yes", false, "确认替换配置中的数据库与附件")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *archive == "" || !*confirmed {
		return fmt.Errorf("请先停止服务，再使用 restore -archive BACKUP.tar.gz -yes 明确确认恢复")
	}
	preserved, err := backup.Restore(context.Background(), *archive, cfg.DatabasePath, cfg.AttachmentDir)
	if err != nil {
		return err
	}
	fmt.Println("数据库与附件已恢复。被替换的原文件保留在：")
	for _, filename := range preserved {
		fmt.Println(filename)
	}
	fmt.Println("请检查恢复结果，再启动服务。")
	return nil
}

func createAdmin(cfg config.Config, args []string) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := flags.String("username", "", "用户名")
	nickname := flags.String("nickname", "", "显示昵称")
	password := flags.String("password", "", "密码（建议使用 ADMIN_PASSWORD）")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *password == "" {
		*password = os.Getenv("ADMIN_PASSWORD")
	}
	if len([]rune(*username)) < 2 || *nickname == "" || !security.ValidPassword(*password) {
		return fmt.Errorf("用户名至少 2 个字符，昵称不能为空，密码至少 10 个字符")
	}
	return withDB(cfg, func(db *sql.DB) error {
		if err := database.Migrate(context.Background(), db); err != nil {
			return err
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin'").Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("管理员已存在，请通过邀请机制添加成员")
		}
		hash, err := security.HashPassword(*password)
		if err != nil {
			return err
		}
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = db.Exec("INSERT INTO users(id,username,nickname,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,'admin',?,?)", security.ID(), *username, *nickname, hash, timestamp, timestamp)
		if err == nil {
			fmt.Println("初始管理员已创建")
		}
		return err
	})
}
func resetPassword(cfg config.Config, args []string) error {
	flags := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	username := flags.String("username", "", "用户名")
	password := flags.String("password", "", "新密码（建议使用 RESET_PASSWORD）")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *password == "" {
		*password = os.Getenv("RESET_PASSWORD")
	}
	if !security.ValidPassword(*password) {
		return fmt.Errorf("新密码至少 10 个字符")
	}
	return withDB(cfg, func(db *sql.DB) error {
		if err := database.Migrate(context.Background(), db); err != nil {
			return err
		}
		hash, err := security.HashPassword(*password)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		result, err := tx.Exec("UPDATE users SET password_hash=?,updated_at=? WHERE username=? COLLATE NOCASE", hash, time.Now().UTC().Format(time.RFC3339Nano), *username)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return fmt.Errorf("用户不存在")
		}
		if _, err = tx.Exec("UPDATE sessions SET revoked_at=? WHERE user_id=(SELECT id FROM users WHERE username=? COLLATE NOCASE) AND revoked_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), *username); err != nil {
			return err
		}
		if err = tx.Commit(); err == nil {
			fmt.Println("密码已重置，所有旧会话已撤销")
		}
		return err
	})
}
