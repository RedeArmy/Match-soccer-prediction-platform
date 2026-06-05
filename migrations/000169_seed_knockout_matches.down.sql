-- NOTE: match_code column (added by 000165) must still be present when
-- rolling back this migration.
DELETE FROM matches
WHERE match_code IN (
    'M73',  'M74',  'M75',  'M76',  'M77',  'M78',  'M79',  'M80',
    'M81',  'M82',  'M83',  'M84',  'M85',  'M86',  'M87',  'M88',
    'M89',  'M90',  'M91',  'M92',  'M93',  'M94',  'M95',  'M96',
    'M97',  'M98',  'M99',  'M100', 'M101', 'M102', 'M103', 'M104'
);
