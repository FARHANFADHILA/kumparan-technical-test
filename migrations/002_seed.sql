-- Dua penulis bernama Budi Santoso dengan id berbeda, sengaja untuk menguji filter nama yang tidak unik.
INSERT INTO authors (id, name) VALUES
  ('author-001', 'Budi Santoso'),
  ('author-002', 'Budi Santoso'),
  ('author-003', 'Siti Rahma'),
  ('author-004', 'Kumparan Sindikasi')
ON CONFLICT (id) DO NOTHING;
