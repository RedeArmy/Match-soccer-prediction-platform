-- Seed push digest gate parameters (DT-15).
-- PushDigestGate collapses P2/P3 push burst within a sliding window into a
-- single summary push, preventing notification spam during concurrent matches.
-- Both params are is_runtime=FALSE: the gate is constructed once at worker startup.

INSERT INTO system_params (key, value, default_value, type, category, description, is_runtime)
VALUES
    ('notify.push_digest_window_sec', '300', '300', 'int', 'notify',
     'Sliding-window length in seconds for the push digest gate. P2/P3 pushes beyond the threshold within this window are collapsed into a single digest push.',
     FALSE),
    ('notify.push_digest_threshold', '5', '5', 'int', 'notify',
     'Maximum individual P2/P3 pushes per user per digest window before collapsing to a summary push.',
     FALSE)
ON CONFLICT (key) DO NOTHING;
