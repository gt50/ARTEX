package traffic

import (
	"fmt"
	"net/url"
	"strings"
)

// RedactStoredCredentials sanitizes the inline SQLite capture format offline.
// It refuses legacy files or blobs before touching anything: those need a
// separate migration. The caller must preserve an encrypted recovery backup and
// stop the recording server before opening this maintenance operation.
func (t *Traffic) RedactStoredCredentials() (int, error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	var unsupported int
	if err := t.db.QueryRow(`SELECT COUNT(*) FROM exchanges e LEFT JOIN exchange_bodies b ON b.id=e.id WHERE e.path<>'' OR COALESCE(b.req_blob,'')<>'' OR COALESCE(b.resp_blob,'')<>''`).Scan(&unsupported); err != nil {
		return 0, err
	}
	if unsupported != 0 {
		return 0, fmt.Errorf("refusing migration: %d legacy/blob captures require separate handling", unsupported)
	}
	type record struct {
		rowid                                 int64
		id, target, requestHead, responseHead string
		requestBody, responseBody             []byte
	}
	rows, err := t.db.Query(`SELECT e.rowid,e.id,e.url,b.req_head,b.req_body,b.resp_head,b.resp_body FROM exchanges e JOIN exchange_bodies b ON b.id=e.id`)
	if err != nil {
		return 0, err
	}
	var records []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.rowid, &r.id, &r.target, &r.requestHead, &r.requestBody, &r.responseHead, &r.responseBody); err != nil {
			rows.Close()
			return 0, err
		}
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if _, err := t.db.Exec(`PRAGMA secure_delete=ON`); err != nil {
		return 0, err
	}
	tx, err := t.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if t.fts {
		if _, err := tx.Exec(`DELETE FROM ex_fts`); err != nil {
			return 0, err
		}
	}
	for _, r := range records {
		if u, err := url.Parse(r.target); err == nil {
			r.target = redactURL(u).String()
		}
		r.requestBody = redactBody(r.requestBody, storedContentType(r.requestHead))
		r.responseBody = redactBody(r.responseBody, storedContentType(r.responseHead))
		r.requestHead = redactStoredHeaders(r.requestHead)
		r.responseHead = redactStoredHeaders(r.responseHead)
		if _, err := tx.Exec(`UPDATE exchanges SET url=? WHERE id=?`, r.target, r.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`UPDATE exchange_bodies SET req_head=?,req_body=?,resp_head=?,resp_body=? WHERE id=?`, r.requestHead, r.requestBody, r.responseHead, r.responseBody, r.id); err != nil {
			return 0, err
		}
		if t.fts {
			content := strings.Join([]string{r.target, r.requestHead, string(r.requestBody), r.responseHead, string(r.responseBody)}, "\n")
			if _, err := tx.Exec(`INSERT INTO ex_fts(rowid,content) VALUES(?,?)`, r.rowid, content); err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	// Compact away old SQLite/FTS pages and truncate the WAL so old bytes cannot
	// remain accessible through workspace downloads after logical redaction.
	if _, err := t.db.Exec(`VACUUM`); err != nil {
		return 0, err
	}
	var busy, log, checkpointed int
	if err := t.db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &log, &checkpointed); err != nil {
		return 0, err
	}
	if busy != 0 {
		return 0, fmt.Errorf("capture database still busy during checkpoint")
	}
	return len(records), nil
}

func storedContentType(head string) string {
	for _, line := range strings.Split(head, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Content-Type") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func redactStoredHeaders(head string) string {
	lines := strings.Split(strings.ReplaceAll(head, "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		parts := strings.SplitN(lines[0], " ", 3)
		if len(parts) == 3 {
			if u, err := url.Parse(parts[1]); err == nil {
				parts[1] = redactURL(u).String()
				lines[0] = strings.Join(parts, " ")
			}
		}
	}
	var result []string
	secret := false
	for i, line := range lines {
		if i > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && secret {
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		secret = i > 0 && ok && credentialField(strings.TrimSpace(key))
		if secret {
			line = key + ": [REDACTED]"
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
