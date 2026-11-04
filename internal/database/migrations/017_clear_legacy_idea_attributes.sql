DELETE FROM idea_attributes WHERE idea_id IN (
    SELECT i.id FROM ideas i
    WHERE i.created_at < '2026-11-04T07:39:25Z'
      AND instr(i.manual_fields_json,'"attributes"')=0
      AND NOT EXISTS (
          SELECT 1 FROM ai_tasks t
          WHERE t.idea_id=i.id AND t.status='succeeded'
            AND t.updated_at >= '2026-11-04T07:39:25Z'
      )
);
