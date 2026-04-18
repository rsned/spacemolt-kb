#!/usr/bin/env python3
"""
Find the most remote systems from any station/base in the galaxy.

This script performs a breadth-first search (BFS) from all systems that have
stations/bases to find which systems are the most hops away from civilization.
"""

import sqlite3
import sys

def main():
    db_path = "../spacemolt-knowledge.db"

    try:
        db = sqlite3.connect(db_path)
        cursor = db.cursor()
    except sqlite3.Error as e:
        print(f"Error connecting to database: {e}")
        sys.exit(1)

    print("Finding most remote systems from stations...")
    print("=" * 60)

    # Find all systems with stations
    cursor.execute("""
        SELECT DISTINCT p.system_id
        FROM bases b
        JOIN pois p ON b.poi_id = p.id
    """)
    station_systems = set(row[0] for row in cursor.fetchall())

    print(f"Found {len(station_systems)} systems with stations")

    # Load all connections
    cursor.execute("SELECT from_system, to_system FROM connections")
    connections = {}
    for from_sys, to_sys in cursor.fetchall():
        if from_sys not in connections:
            connections[from_sys] = []
        if to_sys not in connections:
            connections[to_sys] = []
        connections[from_sys].append(to_sys)
        connections[to_sys].append(from_sys)

    # BFS from all station systems
    distance = {}
    queue = []

    # Initialize queue with all station systems
    for system_id in station_systems:
        distance[system_id] = 0
        queue.append(system_id)

    # BFS
    max_distance = 0
    while queue:
        current = queue.pop(0)

        if current in connections:
            for neighbor in connections[current]:
                if neighbor not in distance:
                    distance[neighbor] = distance[current] + 1
                    max_distance = max(max_distance, distance[neighbor])
                    queue.append(neighbor)

    print(f"\nMaximum distance from station: {max_distance} hops")

    # Statistics by distance
    stats = {}
    for sys_id, dist in distance.items():
        stats[dist] = stats.get(dist, 0) + 1

    print("\nSystems by distance from nearest station:")
    for dist in sorted(stats.keys()):
        print(f"  {dist:2d} hops: {stats[dist]:3d} systems")

    print(f"\nTotal systems reachable from stations: {len(distance)}")

    # Find systems at maximum distance
    remote_systems = [sys_id for sys_id, dist in distance.items() if dist == max_distance]

    print(f"\nMost remote systems ({max_distance} hops from nearest station):")
    print("=" * 60)

    # Get detailed info about most remote systems
    placeholders = ','.join(['?' for _ in remote_systems])

    cursor.execute(f"""
        SELECT s.id, s.name, s.empire, s.police_level, s.position_x, s.position_y,
               COUNT(DISTINCT c.to_system) as connections
        FROM systems s
        LEFT JOIN connections c ON s.id = c.from_system
        WHERE s.id IN ({placeholders})
        GROUP BY s.id, s.name, s.empire, s.police_level, s.position_x, s.position_y
        ORDER BY s.name
    """, remote_systems)

    for row in cursor.fetchall():
        sys_id, name, empire, police, pos_x, pos_y, conns = row

        # Determine security level
        security = "Lawless"
        if police >= 60:
            security = "High"
        elif police >= 30:
            security = "Medium"
        elif police > 0:
            security = "Low"

        empire_display = empire if empire else "Unknown/Neutral"

        print(f"\n🌌 {name}")
        print(f"   System ID: {sys_id}")
        print(f"   Empire: {empire_display}")
        print(f"   Security: {security} (police level: {int(police)})")
        print(f"   Position: ({pos_x:.1f}, {pos_y:.1f})")
        print(f"   Connections: {conns}")

    db.close()

if __name__ == "__main__":
    main()
