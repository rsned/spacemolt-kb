// Package kbpoi holds the knowledge-database rules the KB generators share
// for deciding which points of interest to show.
package kbpoi

// Live is a SQL condition on the pois table, which must be aliased p, that is
// false for a point of interest superseded by a rename.
//
// Server v0.625.0 gave 111 asteroid belts and nebulae unique names and new
// IDs. Their old rows were named "<Name> — <System>" (an em dash). The
// knowledge database keeps a row until something overwrites it, so a system
// an agent has scanned since the rename lists the old and the new POI side by
// side, and the old row still carries its resource survey. A row is
// superseded when it has the old naming and the system has been scanned since
// it was last updated (another POI there has a newer tick). Rows in systems
// nobody has revisited stay visible under the old name: they are still the
// latest data available for that system.
const Live = `NOT (p.name LIKE '% — ' || (SELECT s2.name FROM systems s2 WHERE s2.id = p.system_id)
	AND p.last_updated_tick < (SELECT MAX(q.last_updated_tick) FROM pois q WHERE q.system_id = p.system_id))`
