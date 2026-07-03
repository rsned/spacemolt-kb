CREATE TABLE IF NOT EXISTS bill_of_materials (
    target_id      TEXT NOT NULL,
    target_type    TEXT NOT NULL,
    base_item_id   TEXT NOT NULL,
    quantity       INTEGER NOT NULL,
    recipe_path    TEXT,
    has_alternatives BOOLEAN DEFAULT 0,
    PRIMARY KEY (target_id, target_type, base_item_id)
);
