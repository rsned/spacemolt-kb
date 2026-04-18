# Finding Remote Systems

This directory contains tools to find the most remote systems from stations/bases in the galaxy.

## Quick Start

**Recommended method:** Use the Python script
```bash
python3 find_remote_systems.py
```

**Alternative:** Try the SQL queries (may timeout on large databases)
```bash
sqlite3 ../spacemolt-knowledge.db < find_remote_systems.sql
```

## What This Does

Performs a breadth-first search (BFS) from all systems that have stations/bases to find:

1. **Most remote systems** - Systems that require the most jumps to reach from civilization
2. **Distance distribution** - How many systems are at each distance (1 hop, 2 hops, etc.)
3. **System details** - Empire, security level, position, and connection count for remote systems

## Current Results (as of last run)

- **Most remote distance**: 16 hops from nearest station
- **Systems at max distance**: 2 systems
  - **Barnard 44** - Unknown/Neutral, Lawless, Position: (3930.8, 2052.6)
  - **Bellatrix** - Unknown/Neutral, Lawless, Position: (6132.2, -265.3)
- **Total systems with stations**: 21
- **Total systems reachable**: 505 (fully connected galaxy)

## Files

- `find_remote_systems.py` - Python script (recommended, fast and reliable)
- `find_remote_systems.sql` - SQL queries (may timeout on large databases)
- `FIND_REMOTE_SYSTEMS.md` - This documentation file

## Requirements

- Python 3.x with sqlite3 module
- Access to `../spacemolt-knowledge.db`
- Tables used: `systems`, `connections`, `bases`, `pois`

## How It Works

The algorithm performs BFS from all station systems simultaneously:

1. **Initialize**: Mark all systems with stations as distance 0
2. **Expand**: For each frontier system, add all unvisited neighbors at distance+1
3. **Track**: Keep expanding until all reachable systems are visited
4. **Find Maximum**: Identify systems at the maximum distance

This gives you the "edge of explored space" - systems that are the hardest to reach from civilization.
