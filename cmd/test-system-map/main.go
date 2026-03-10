package main

import (
	"fmt"
	"os"
)

type POI struct {
	ID, Type, Class, Name string
	PositionX, PositionY  float64
}

type System struct {
	ID    string
	Name  string
	POIs  []POI
}

func main() {
	systems := []struct {
		name   string
		system System
	}{
		{
			name: "o-hypergiant",
			system: System{
				ID:   "test-o-hypergiant",
				Name: "O-type Hypergiant System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "O3 Ia", Name: "Hypergiant", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "jovian", Name: "Gas Giant", PositionX: 5, PositionY: 0},
				},
			},
		},
		{
			name: "g2v-sun",
			system: System{
				ID:   "test-g2v",
				Name: "G-type Main Sequence System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "G2 V", Name: "Sol-like", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "terran", Name: "Earth-like", PositionX: 1, PositionY: 0},
				},
			},
		},
		{
			name: "m-dwarf",
			system: System{
				ID:   "test-m-dwarf",
				Name: "M-type Dwarf System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "M9 V", Name: "Red Dwarf", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "rocky", Name: "Rocky World", PositionX: 0.5, PositionY: 0},
				},
			},
		},
		{
			name: "brown-dwarf",
			system: System{
				ID:   "test-brown-dwarf",
				Name: "Brown Dwarf System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "L5 V", Name: "Brown Dwarf", PositionX: 0, PositionY: 0},
				},
			},
		},
		{
			name: "mixed-planets",
			system: System{
				ID:   "test-mixed",
				Name: "Mixed Planet System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "G2 V", Name: "Star", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "jovian", Name: "Gas Giant", PositionX: 3, PositionY: 0},
					{ID: "p2", Type: "planet", Class: "ice_giant", Name: "Ice Giant", PositionX: 5, PositionY: 0},
					{ID: "p3", Type: "planet", Class: "terran", Name: "Terran", PositionX: 1.5, PositionY: 0},
				},
			},
		},
	}

	for _, tc := range systems {
		filename := fmt.Sprintf("test-%s.html", tc.name)
		f, _ := os.Create(filename)
		f.WriteString(fmt.Sprintf("<!-- %s -->\n", tc.name))
		f.WriteString(fmt.Sprintf("<!-- System: %s -->\n", tc.system.Name))
		for _, poi := range tc.system.POIs {
			f.WriteString(fmt.Sprintf("<!-- POI: %s (%s) class: %s -->\n", poi.Name, poi.Type, poi.Class))
		}
		f.Close()
		fmt.Printf("Generated %s\n", filename)
	}
}
