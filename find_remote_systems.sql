-- Find most remote systems from stations
-- This query finds systems that are the most hops away from any system with a station/base
--
-- NOTE: Due to SQLite limitations with recursive CTEs and the size of the database,
-- these queries may timeout or hit memory limits. For reliable results, use the Python
-- script instead: python3 find_remote_systems.py
--
-- If you want to try the SQL anyway, run: sqlite3 ../spacemolt-knowledge.db < find_remote_systems.sql
WITH RECURSIVE bfs AS (
  -- Start with all station systems
  SELECT
    s.id,
    s.name,
    0 as hops
  FROM systems s
  WHERE s.id IN (
    SELECT DISTINCT p.system_id
    FROM bases b
    JOIN pois p ON b.poi_id = p.id
  )

  UNION ALL

  -- Expand one hop at a time
  SELECT
    dest.id,
    dest.name,
    src.hops + 1
  FROM bfs src
  JOIN connections c ON src.id = c.from_system
  JOIN systems dest ON c.to_system = dest.id
  WHERE src.hops < 20  -- Limit to prevent infinite loops
)
SELECT
  hops,
  COUNT(*) as system_count
FROM bfs
GROUP BY hops
ORDER BY hops;

-- Find the maximum distance
WITH RECURSIVE bfs AS (
  SELECT
    s.id,
    s.name,
    0 as hops
  FROM systems s
  WHERE s.id IN (
    SELECT DISTINCT p.system_id
    FROM bases b
    JOIN pois p ON b.poi_id = p.id
  )

  UNION ALL

  SELECT
    dest.id,
    dest.name,
    src.hops + 1
  FROM bfs src
  JOIN connections c ON src.id = c.from_system
  JOIN systems dest ON c.to_system = dest.id
  WHERE src.hops < 20
)
SELECT MAX(hops) as maximum_hops_from_station FROM bfs;

-- Find the actual most remote systems
WITH RECURSIVE bfs AS (
  SELECT
    s.id,
    s.name,
    0 as hops
  FROM systems s
  WHERE s.id IN (
    SELECT DISTINCT p.system_id
    FROM bases b
    JOIN pois p ON b.poi_id = p.id
  )

  UNION ALL

  SELECT
    dest.id,
    dest.name,
    src.hops + 1
  FROM bfs src
  JOIN connections c ON src.id = c.from_system
  JOIN systems dest ON c.to_system = dest.id
  WHERE src.hops < 20
)
SELECT
  b.name as system_name,
  b.id as system_id,
  s.empire,
  s.police_level,
  s.position_x,
  s.position_y,
  COUNT(DISTINCT c.to_system) as connections,
  b.hops
FROM bfs b
JOIN systems s ON b.id = s.id
LEFT JOIN connections c ON b.id = c.from_system
WHERE b.hops = (SELECT MAX(hops) FROM bfs)
GROUP BY b.id, b.name, s.empire, s.police_level, s.position_x, s.position_y, b.hops
ORDER BY b.name;
