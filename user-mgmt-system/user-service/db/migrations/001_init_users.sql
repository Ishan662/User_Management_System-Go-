CREATE TYPE user_status AS ENUM ('Active', 'Inactive', 'Suspended');

CREATE TABLE users (
    user_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT NOT NULL,
    last_name  TEXT NOT NULL,
    email      TEXT NOT NULL UNIQUE,
    phone      TEXT,
    age        INTEGER,
    status     user_status NOT NULL DEFAULT 'Active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);