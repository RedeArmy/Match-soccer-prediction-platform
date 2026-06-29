-- Restore original (incorrect) descriptions from migration 000205.
UPDATE tournament_slots SET description = 'Ganador R32 M01' WHERE label = 'r16_01_a';
UPDATE tournament_slots SET description = 'Ganador R32 M02' WHERE label = 'r16_01_b';
UPDATE tournament_slots SET description = 'Ganador R32 M03' WHERE label = 'r16_02_a';
UPDATE tournament_slots SET description = 'Ganador R32 M04' WHERE label = 'r16_02_b';
UPDATE tournament_slots SET description = 'Ganador R32 M05' WHERE label = 'r16_03_a';
UPDATE tournament_slots SET description = 'Ganador R32 M09' WHERE label = 'r16_05_a';
UPDATE tournament_slots SET description = 'Ganador R32 M10' WHERE label = 'r16_05_b';
UPDATE tournament_slots SET description = 'Ganador R32 M11' WHERE label = 'r16_06_a';
UPDATE tournament_slots SET description = 'Ganador R32 M12' WHERE label = 'r16_06_b';
UPDATE tournament_slots SET description = 'Ganador R32 M13' WHERE label = 'r16_07_a';
UPDATE tournament_slots SET description = 'Ganador R32 M14' WHERE label = 'r16_07_b';
UPDATE tournament_slots SET description = 'Ganador R32 M15' WHERE label = 'r16_08_a';
UPDATE tournament_slots SET description = 'Ganador R32 M16' WHERE label = 'r16_08_b';

UPDATE tournament_slots SET description = 'Ganador R16 M03' WHERE label = 'qf_02_a';
UPDATE tournament_slots SET description = 'Ganador R16 M04' WHERE label = 'qf_02_b';
UPDATE tournament_slots SET description = 'Ganador R16 M05' WHERE label = 'qf_03_a';
UPDATE tournament_slots SET description = 'Ganador R16 M06' WHERE label = 'qf_03_b';
