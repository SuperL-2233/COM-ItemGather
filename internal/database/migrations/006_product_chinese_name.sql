ALTER TABLE products RENAME COLUMN final_brand_name TO chinese_name;
UPDATE products SET chinese_name=internal_name WHERE chinese_name='';
ALTER TABLE products DROP COLUMN internal_name;
