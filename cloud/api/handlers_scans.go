package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

// referenceTail keeps the last four digits of a customer reference so a
// merchant can recognise a repeat visitor ("···4471") without Cloud storing a
// phone book. Non-digits are dropped first, so it works for phone numbers in
// any format; anything shorter than four digits yields "" rather than a value
// that could identify a single person.
func referenceTail(ref string) string {
	digits := make([]byte, 0, len(ref))
	for i := 0; i < len(ref); i++ {
		if ref[i] >= '0' && ref[i] <= '9' {
			digits = append(digits, ref[i])
		}
	}
	if len(digits) < 4 {
		return ""
	}
	return string(digits[len(digits)-4:])
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

const maxScanPage = 200

// handleListScans answers the merchant's real question: who scanned, when,
// from where, and whether it is already anchored on-chain. Scoped to the
// caller's org/env; the customer is identified only by the last four digits.
func (s *server) handleListScans(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(w, 404, "campaign not found")
		return
	}
	if _, err := s.campaignByID(a, id); err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	limit := int64(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || limit < 1 || limit > maxScanPage {
			writeProblem(w, 400, "limit must be between 1 and 200")
			return
		}
	}
	cursor := int64(0) // id-descending cursor: rows strictly older than this id
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			writeProblem(w, 400, "cursor must be a non-negative integer")
			return
		}
	}

	query := `SELECT e.id, sc.code, e.count, e.created_at,
	  COALESCE(e.customer_tail,''), e.lat, e.lon, e.accuracy_m,
	  COALESCE(e.evidence_type,''), e.committed_period,
	  COALESCE(er.leaf_hash,''), COALESCE(t.tx_hash,'')
	  FROM shared_events e
	  JOIN shared_codes sc ON sc.id = e.shared_code_id
	  LEFT JOIN event_receipts er ON er.event_id = e.id
	  LEFT JOIN tallies t ON t.shared_code_id = e.shared_code_id AND t.period = e.committed_period
	  WHERE sc.campaign_id = ?`
	args := []any{id}
	if cursor > 0 {
		query += ` AND e.id < ?`
		args = append(args, cursor)
	}
	query += ` ORDER BY e.id DESC LIMIT ?`
	args = append(args, limit+1) // one extra row tells us whether more remain

	rows, err := s.db.Query(query, args...)
	if err != nil {
		writeInternal(w, err, "list scans")
		return
	}
	defer rows.Close()
	scans := []map[string]any{}
	var lastID int64
	more := false
	for rows.Next() {
		if int64(len(scans)) == limit {
			more = true
			break
		}
		var eventID, count, createdAt int64
		var code, tail, evidence, leaf, txHash string
		var lat, lon, accuracy sql.NullFloat64
		var period sql.NullInt64
		if err := rows.Scan(&eventID, &code, &count, &createdAt, &tail,
			&lat, &lon, &accuracy, &evidence, &period, &leaf, &txHash); err != nil {
			writeInternal(w, err, "decode scan")
			return
		}
		row := map[string]any{
			"id": eventID, "code": code, "count": count, "created_at": createdAt,
			"customer_tail": tail, "evidence_type": evidence, "leaf_hash": leaf,
		}
		if lat.Valid && lon.Valid {
			row["lat"], row["lon"] = lat.Float64, lon.Float64
			if accuracy.Valid {
				row["accuracy_m"] = accuracy.Float64
			}
		}
		switch {
		case !period.Valid:
			row["anchor"] = "pending"
		case period.Int64 == -1:
			// pre-audit rows preserved but never retroactively signed
			row["anchor"] = "legacy"
		default:
			row["anchor"] = "committed"
			row["period"] = period.Int64
			row["period_label"] = periodLabel(uint64(period.Int64))
			if txHash != "" {
				row["tx_hash"] = txHash
			}
		}
		scans = append(scans, row)
		lastID = eventID
	}
	if err := rows.Err(); err != nil {
		writeInternal(w, err, "read scans")
		return
	}

	// Summary over the whole campaign, not just this page: the headline numbers
	// a merchant looks at first.
	var total, distinct, located int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0),
	  COUNT(DISTINCT NULLIF(e.customer_ref,'')),
	  COALESCE(SUM(CASE WHEN e.lat IS NOT NULL THEN 1 ELSE 0 END),0)
	  FROM shared_events e JOIN shared_codes sc ON sc.id = e.shared_code_id
	  WHERE sc.campaign_id = ?`, id).Scan(&total, &distinct, &located); err != nil {
		writeInternal(w, err, "summarize scans")
		return
	}
	out := map[string]any{
		"scans": scans, "limit": limit,
		"summary": map[string]any{
			"total_scans": total, "distinct_customers": distinct, "with_location": located,
		},
	}
	if more {
		out["next_cursor"] = lastID
	}
	writeJSON(w, 200, out)
}
