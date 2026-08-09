package store

// LibIDs returns catalog LIBIDs. If aliveOnly, only non-deleted rows are included.
func (s *Store) LibIDs(aliveOnly bool) (map[string]struct{}, error) {
	q := `SELECT lib_id FROM books WHERE lib_id != ''`
	if aliveOnly {
		q += ` AND deleted = 0`
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make(map[string]struct{}, 1<<20)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}
