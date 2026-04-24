UPDATE users SET role = 'player' WHERE role = 'user';

ALTER TABLE users
    DROP CONSTRAINT chk_users_role,
    ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'player')),
    ALTER COLUMN role SET DEFAULT 'player';
