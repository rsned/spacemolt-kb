package systemmap

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
)

// shipType defines a ship icon with its symbol ID and allowed route contexts.
type shipType struct {
	id   string  // symbol ID in SVG defs
	size float64 // bounding box size in pixels
}

var shipTypes = []shipType{
	{id: "ship-shuttle", size: 10},
	{id: "ship-freighter", size: 16},
	{id: "ship-hauler", size: 14},
	{id: "ship-corvette", size: 12},
	{id: "ship-scow", size: 14},
	{id: "ship-raider", size: 12},
}

// Ship indices for route-type selection.
const (
	shipShuttle   = 0
	shipFreighter = 1
	shipHauler    = 2
	shipCorvette  = 3
	shipScow      = 4
	shipRaider    = 5
)

// shipsForRoute returns the allowed ship indices for a given route type.
func shipsForRoute(rt routeType) []int {
	switch rt {
	case routeGateGate:
		return []int{shipShuttle, shipCorvette}
	case routeStationGate:
		return []int{shipCorvette, shipShuttle, shipFreighter}
	case routeGateResource:
		return []int{shipScow, shipShuttle}
	case routeStationResource:
		return []int{shipFreighter, shipHauler, shipScow}
	case routeGateSun:
		return []int{shipRaider}
	}
	return []int{shipShuttle}
}

// routeType classifies a ship route.
type routeType int

const (
	routeGateGate        routeType = iota
	routeStationGate               // station ↔ jump gate
	routeGateResource              // gate ↔ resource (no station)
	routeStationResource           // station ↔ resource
	routeGateSun                   // gate ↔ sun (pirate dead-end)
)

// shipRoute holds the endpoints and metadata for one animated route.
type shipRoute struct {
	rt         routeType
	ax, ay     float64 // endpoint A (SVG coords)
	bx, by     float64 // endpoint B (SVG coords)
	beginDelay float64 // stagger offset in seconds
}

// renderShipSymbols returns SVG <symbol> definitions for all 6 ship icons.
// Each symbol is centered on (0,0) and oriented pointing right (+X).
func renderShipSymbols() string {
	var b strings.Builder

	// 1. Shuttle — small pointed wedge.
	b.WriteString(`<symbol id="ship-shuttle" viewBox="-5 -5 10 10">`)
	b.WriteString(`<polygon points="5,0 -4,-3.5 -2,0 -4,3.5" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	// 2. Freighter — wide rectangular hull with stubby wings.
	b.WriteString(`<symbol id="ship-freighter" viewBox="-8 -8 16 16">`)
	b.WriteString(`<polygon points="7,0 3,-3 -5,-3 -7,-6 -7,6 -5,3 3,3" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	// 3. Hauler — long narrow body with boxy rear.
	b.WriteString(`<symbol id="ship-hauler" viewBox="-7 -7 14 14">`)
	b.WriteString(`<polygon points="7,0 4,-2 -5,-2 -5,-4 -7,-4 -7,4 -5,4 -5,2 4,2" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	// 4. Corvette — sleek pointed nose with swept wings.
	b.WriteString(`<symbol id="ship-corvette" viewBox="-6 -6 12 12">`)
	b.WriteString(`<polygon points="6,0 1,-2 -3,-2 -5,-5 -4,-2 -5,0 -4,2 -5,5 -3,2 1,2" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	// 5. Scow — squat asymmetric trapezoid.
	b.WriteString(`<symbol id="ship-scow" viewBox="-7 -7 14 14">`)
	b.WriteString(`<polygon points="5,-1 5,2 -4,4 -6,3 -6,-2 -4,-3 5,-1" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	// 6. Raider — angular aggressive wedge with notched wings.
	b.WriteString(`<symbol id="ship-raider" viewBox="-6 -6 12 12">`)
	b.WriteString(`<polygon points="6,0 0,-2 -2,-1 -4,-5 -3,-1 -5,0 -3,1 -4,5 -2,1 0,2" fill="#8b95ab" opacity="0.6"/>`)
	b.WriteString(`</symbol>`)

	return b.String()
}

// generateShipRoutes produces the complete SVG block for ambient ship traffic
// in a system: symbol definitions (if not already emitted) and animated ship
// groups traveling between POI pairs.
func generateShipRoutes(sys *System, allSystems map[string]*System, scale, cx, cy, gateRadius, vbX, vbW, visTop, visBottom float64) string {
	if len(sys.Connections) == 0 {
		return "" // no gates, no traffic
	}

	// Compute gate SVG positions.
	type gatePos struct {
		x, y float64
		name string
	}
	var gates []gatePos
	for _, conn := range sys.Connections {
		angle := computeGateAngle(sys, conn.SystemID, allSystems)
		gx := cx + gateRadius*math.Cos(angle)
		gy := cy - gateRadius*math.Sin(angle)
		gx = math.Max(vbX+30, math.Min(vbX+vbW-30, gx))
		gy = math.Max(visTop, math.Min(visBottom, gy))
		gates = append(gates, gatePos{x: gx, y: gy, name: conn.Name})
	}

	// Collect POI positions by type.
	type poiPos struct {
		x, y float64
	}
	var stations, resources []poiPos
	var sunPos *poiPos
	for _, poi := range sys.POIs {
		px := cx + poi.PositionX*scale
		py := cy - poi.PositionY*scale
		switch poi.Type {
		case "station":
			stations = append(stations, poiPos{x: px, y: py})
		case "asteroid_belt", "ice_field", "gas_cloud":
			resources = append(resources, poiPos{x: px, y: py})
		case "sun":
			sunPos = &poiPos{x: px, y: py}
		}
	}

	sec := sys.Security
	rng := rand.New(rand.NewPCG(poiSeed(sys.ID), poiSeed(sys.ID)^0xf1ee7))

	var routes []shipRoute
	stagger := 0.0

	// Helper to pick a count from a range based on security band.
	pickCount := func(lo, mid, hi int) int {
		var base int
		switch {
		case sec <= 30:
			base = lo
		case sec <= 70:
			base = mid
		default:
			base = hi
		}
		// Add some randomness: ±1 but floor at original lo.
		if base > 1 && rng.IntN(3) == 0 {
			base--
		} else if rng.IntN(3) == 0 {
			base++
		}
		if base < lo {
			base = lo
		}
		return base
	}

	// Dead-end pirate stronghold: Gate↔Sun.
	if len(gates) == 1 && sec <= 30 && sunPos != nil && len(stations) == 0 && len(resources) == 0 {
		routes = append(routes, shipRoute{
			rt: routeGateSun,
			ax: gates[0].x, ay: gates[0].y,
			bx: sunPos.x, by: sunPos.y,
			beginDelay: stagger,
		})
		return renderAnimatedRoutes(routes, rng)
	}

	// Gate↔Gate routes.
	if len(gates) >= 2 {
		n := pickCount(1, 2, 6)
		for range n {
			g1 := rng.IntN(len(gates))
			g2 := (g1 + 1 + rng.IntN(len(gates)-1)) % len(gates)
			routes = append(routes, shipRoute{
				rt: routeGateGate,
				ax: gates[g1].x, ay: gates[g1].y,
				bx: gates[g2].x, by: gates[g2].y,
				beginDelay: stagger,
			})
			stagger += 2 + rng.Float64()*4
		}
	}

	// Station↔Gate routes.
	if len(stations) > 0 {
		n := pickCount(0, 1, 4)
		for range n {
			st := stations[rng.IntN(len(stations))]
			gt := gates[rng.IntN(len(gates))]
			routes = append(routes, shipRoute{
				rt: routeStationGate,
				ax: st.x, ay: st.y,
				bx: gt.x, by: gt.y,
				beginDelay: stagger,
			})
			stagger += 2 + rng.Float64()*3
		}
	}

	hasStation := len(stations) > 0
	hasResource := len(resources) > 0

	// Gate↔Resource (only when no station).
	if !hasStation && hasResource {
		n := pickCount(0, 1, 4)
		for range n {
			res := resources[rng.IntN(len(resources))]
			gt := gates[rng.IntN(len(gates))]
			routes = append(routes, shipRoute{
				rt: routeGateResource,
				ax: gt.x, ay: gt.y,
				bx: res.x, by: res.y,
				beginDelay: stagger,
			})
			stagger += 2 + rng.Float64()*3
		}
	}

	// Station↔Resource (when both exist — heavier traffic).
	if hasStation && hasResource {
		n := pickCount(0, 2, 8)
		for range n {
			st := stations[rng.IntN(len(stations))]
			res := resources[rng.IntN(len(resources))]
			routes = append(routes, shipRoute{
				rt: routeStationResource,
				ax: st.x, ay: st.y,
				bx: res.x, by: res.y,
				beginDelay: stagger,
			})
			stagger += 1 + rng.Float64()*3
		}
	}

	if len(routes) == 0 {
		return ""
	}

	return renderAnimatedRoutes(routes, rng)
}

// renderAnimatedRoutes produces SVG for all ship route animations.
func renderAnimatedRoutes(routes []shipRoute, rng *rand.Rand) string {
	var b strings.Builder

	for i, route := range routes {
		allowed := shipsForRoute(route.rt)
		// Pick ship for outbound trip.
		outShip := shipTypes[allowed[rng.IntN(len(allowed))]]
		// Pick ship for return trip (may differ).
		retShip := shipTypes[allowed[rng.IntN(len(allowed))]]

		// Compute curved path between A and B.
		// Perpendicular offset for curve; alternate direction per route.
		dx := route.bx - route.ax
		dy := route.by - route.ay
		dist := math.Hypot(dx, dy)
		if dist < 10 {
			continue
		}

		nx := dx / dist
		ny := dy / dist
		perpX := -ny
		perpY := nx

		// Curve magnitude: ~15% of distance, alternating sides.
		curveMag := dist * 0.15
		if i%2 == 1 {
			curveMag = -curveMag
		}

		// Quadratic bezier control point.
		midX := (route.ax+route.bx)/2 + perpX*curveMag
		midY := (route.ay+route.by)/2 + perpY*curveMag

		// Forward path: A → B.
		pathID := fmt.Sprintf("route-%d-fwd", i)
		b.WriteString(fmt.Sprintf(
			`<path id="%s" d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" fill="none" stroke="none"/>`,
			pathID, route.ax, route.ay, midX, midY, route.bx, route.by))

		// Reverse path: B → A.
		pathIDRev := fmt.Sprintf("route-%d-rev", i)
		b.WriteString(fmt.Sprintf(
			`<path id="%s" d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" fill="none" stroke="none"/>`,
			pathIDRev, route.bx, route.by, midX, midY, route.ax, route.ay))

		// Outbound ship (A→B): begin at stagger, 20s travel, then invisible.
		// Cycle: 20s travel + 10s wait + 20s return + 10s wait = 60s total.
		beginOut := route.beginDelay
		b.WriteString(renderAnimatedShip(outShip, pathID, beginOut, i, "out"))

		// Return ship (B→A): offset by 30s from outbound start.
		beginRet := route.beginDelay + 30
		b.WriteString(renderAnimatedShip(retShip, pathIDRev, beginRet, i, "ret"))
	}

	return b.String()
}

// renderAnimatedShip renders a single ship icon animating along a path.
func renderAnimatedShip(ship shipType, pathID string, beginDelay float64, routeIdx int, dir string) string {
	var b strings.Builder

	s := ship.size
	uid := fmt.Sprintf("%s-%d-%s", ship.id, routeIdx, dir)

	b.WriteString(fmt.Sprintf(`<g id="%s">`, uid))

	// The ship icon.
	b.WriteString(fmt.Sprintf(
		`<use href="#%s" width="%.0f" height="%.0f" x="%.0f" y="%.0f">`,
		ship.id, s, s, -s/2, -s/2))

	// Motion along the path.
	b.WriteString(fmt.Sprintf(
		`<animateMotion dur="20s" begin="%.1fs" repeatCount="indefinite" rotate="auto" fill="freeze">`,
		beginDelay))
	b.WriteString(fmt.Sprintf(`<mpath href="#%s"/>`, pathID))
	b.WriteString(`</animateMotion>`)

	b.WriteString(`</use>`)

	// Opacity: visible during travel (20s), invisible during wait+return+wait (40s).
	// Keyframes at 0/60: visible=0→1 over 1s, hold for 18s, fade 1→0 over 1s, hold 0 for 40s.
	// Using proportional keyTimes for a 60s cycle.
	b.WriteString(fmt.Sprintf(
		`<animate attributeName="opacity" dur="60s" begin="%.1fs" repeatCount="indefinite" `+
			`keyTimes="0;0.017;0.3;0.333;1" values="0;1;1;0;0"/>`,
		beginDelay))

	b.WriteString(`</g>`)

	return b.String()
}
