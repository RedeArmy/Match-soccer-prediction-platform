ALTER TABLE matches
    ADD COLUMN win_method TEXT CHECK (win_method IN ('normal', 'extra_time', 'penalties'));

ALTER TABLE predictions
    ADD COLUMN predicted_win_method TEXT CHECK (predicted_win_method IN ('normal', 'extra_time', 'penalties'));
