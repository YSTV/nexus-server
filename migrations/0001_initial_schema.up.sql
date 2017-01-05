CREATE TABLE streams (
    display_name string,
    is_public bool,
    start_at time,
    end_at time,
    stream_name string NOT NULL,
    key string NOT NULL
);

CREATE UNIQUE INDEX stream_name_unique ON streams (stream_name);
