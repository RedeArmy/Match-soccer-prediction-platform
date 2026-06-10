-- Migration 000180: seed tournament.general_entry_fee_cents and
-- tournament.round_entry_fee_cents system parameters.
--
-- These two parameters control the entry fees for the two paid sub-modes of the
-- hybrid quiniela. Groups are created as free (is_premium=false, entry_fee=0) by
-- default; the group owner may upgrade to premium and enable either or both modes
-- once the group reaches the minimum member count.
--
--   general_entry_fee_cents — single fee charged once to join the overall
--     competition pool. Prizes follow the existing WinnerCount tier table.
--     Default: 3000 (Q30.00 GTQ).
--
--   round_entry_fee_cents — fee charged per jornada/round. Only members who
--     have paid for the active round count toward that round's prize distribution
--     (always 1st and 2nd place only).
--     Default: 1500 (Q15.00 GTQ).
--
-- Both are is_runtime=TRUE: operators may adjust fees between rounds without a
-- process restart; the new value takes effect within the 30-second cache window.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'tournament.general_entry_fee_cents',
        '3000',
        '3000',
        'int',
        'tournament',
        TRUE,
        'Entry fee in GTQ minor units (cents) charged once per member to join the '
        'overall-competition prize pool when a group operates in general mode '
        '(is_premium=true, mode_general=true). Default: 3000 (Q30.00).'
    ),
    (
        'tournament.round_entry_fee_cents',
        '1500',
        '1500',
        'int',
        'tournament',
        TRUE,
        'Entry fee in GTQ minor units (cents) charged per round/jornada when a group '
        'operates in per-round mode (is_premium=true, mode_round=true). Only members '
        'who have paid for the active round are included in that round''s prize '
        'distribution (1st and 2nd place only). Default: 1500 (Q15.00).'
    )
ON CONFLICT (key) DO NOTHING;
