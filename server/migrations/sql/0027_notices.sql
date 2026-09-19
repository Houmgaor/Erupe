-- Login notices managed at runtime through the admin API (POST /v2/admin/notices).
-- Shown at sign-in after the static LoginNotices from config.json, so an
-- operator can announce maintenance or an event without editing the config
-- and restarting. A NULL expires_at means "until deleted".

CREATE TABLE IF NOT EXISTS public.notices (
    id         SERIAL PRIMARY KEY,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);
