-- Retire the idea-only fields, including historical copies and manual locks.
UPDATE ideas SET manual_fields_json = (
    SELECT json_group_array(value) FROM json_each(ideas.manual_fields_json)
    WHERE value NOT IN ('name_candidates', 'tags', 'attributes')
);

UPDATE revisions SET
    before_data = CASE WHEN before_data IS NULL THEN NULL
        ELSE json_remove(before_data, '$.name_candidates', '$.tags', '$.attributes') END,
    after_data = json_remove(after_data, '$.name_candidates', '$.tags', '$.attributes')
WHERE entity_type = 'idea';

UPDATE revisions SET before_data = json_set(before_data, '$.manual_fields', json((
    SELECT json_group_array(value) FROM json_each(revisions.before_data, '$.manual_fields')
    WHERE value NOT IN ('name_candidates', 'tags', 'attributes')
)))
WHERE entity_type = 'idea' AND json_type(before_data, '$.manual_fields') = 'array';

UPDATE revisions SET after_data = json_set(after_data, '$.manual_fields', json((
    SELECT json_group_array(value) FROM json_each(revisions.after_data, '$.manual_fields')
    WHERE value NOT IN ('name_candidates', 'tags', 'attributes')
)))
WHERE entity_type = 'idea' AND json_type(after_data, '$.manual_fields') = 'array';

DROP TABLE idea_attributes;
DROP TABLE idea_tags;
DROP TABLE tags;
ALTER TABLE ideas DROP COLUMN name_candidates_json;
ALTER TABLE ideas DROP COLUMN tags_json;
