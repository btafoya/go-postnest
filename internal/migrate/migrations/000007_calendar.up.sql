CREATE TABLE calendars (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id   UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    color       VARCHAR(16) NOT NULL DEFAULT '#4285f4',
    description TEXT NOT NULL DEFAULT '',
    ctag        BIGINT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX calendars_domain_user_idx ON calendars (domain_id, user_id);

CREATE TABLE calendar_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_id  UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    domain_id    UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    uid          VARCHAR(255) NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    location     TEXT NOT NULL DEFAULT '',
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    all_day      BOOLEAN NOT NULL DEFAULT false,
    rrule        TEXT NOT NULL DEFAULT '',
    status       VARCHAR(32) NOT NULL DEFAULT 'CONFIRMED',
    organizer    VARCHAR(320) NOT NULL DEFAULT '',
    attendees    JSONB NOT NULL DEFAULT '[]',
    sequence     INT NOT NULL DEFAULT 0,
    etag         VARCHAR(64) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (calendar_id, uid)
);

CREATE INDEX calendar_events_cal_time_idx ON calendar_events (calendar_id, starts_at);
CREATE INDEX calendar_events_user_idx ON calendar_events (domain_id, user_id);
