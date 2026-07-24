MERGE INTO baseline AS target
USING staged_baseline AS source
ON target.id = source.id
WHEN MATCHED THEN UPDATE SET id = source.id;
