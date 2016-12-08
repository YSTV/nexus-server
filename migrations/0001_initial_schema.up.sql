CREATE TABLE "streams" (
    id INTEGER PRIMARY KEY,
    name VARCHAR(255),
    is_public BOOLEAN,
    start_at DATETIME,
    end_at DATETIME
);
