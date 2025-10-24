CREATE TABLE IF NOT EXISTS persons (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  age INTEGER,
  address TEXT,
  work TEXT
);
