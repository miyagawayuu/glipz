-- One-time migration from legacy posts.object_key column to object_keys array.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS object_keys TEXT[];

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'posts' AND column_name = 'object_key'
  ) THEN
    UPDATE posts SET object_keys = ARRAY[object_key]::text[]
      WHERE object_keys IS NULL AND object_key IS NOT NULL;
  END IF;
  UPDATE posts SET object_keys = ARRAY['legacy']::text[]
    WHERE object_keys IS NULL;
END $$;

ALTER TABLE posts ALTER COLUMN object_keys SET NOT NULL;

ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_object_keys_len;
ALTER TABLE posts ADD CONSTRAINT posts_object_keys_len CHECK (cardinality(object_keys) BETWEEN 1 AND 4);

ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_video_single;
ALTER TABLE posts ADD CONSTRAINT posts_video_single CHECK (media_type <> 'video' OR cardinality(object_keys) = 1);

ALTER TABLE posts DROP COLUMN IF EXISTS object_key;
