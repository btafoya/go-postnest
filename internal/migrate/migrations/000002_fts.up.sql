CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE OR REPLACE FUNCTION messages_update_search_vector()
RETURNS TRIGGER AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('english', coalesce(NEW.subject, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(NEW.from_address, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.from_name, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.plain_text, '')), 'C') ||
    setweight(to_tsvector('simple', coalesce(NEW.to_addresses::text, '')), 'D');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
