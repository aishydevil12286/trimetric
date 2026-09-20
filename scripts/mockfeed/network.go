package main

import (
	"fmt"
	"math"
	"math/rand"
)

// Connaught Place, the centre the generated network radiates from.
const (
	centreLat = 28.6328
	centreLng = 77.2197
)

// Route colours, chosen to stay distinguishable against a light basemap.
var routeColors = []string{
	"E4572E", "17BEBB", "2E86AB", "A23B72", "F18F01",
	"5B8C5A", "C73E1D", "7768AE", "3F8EFC", "D95D39",
}

// Locality names used to label stops. Real Delhi place names make the stop
// list legible; the coordinates they are attached to are invented.
var localities = []string{
	"Connaught Place", "Karol Bagh", "Rajouri Garden", "Janakpuri", "Dwarka Mor",
	"Uttam Nagar", "Tilak Nagar", "Subhash Nagar", "Kirti Nagar", "Patel Nagar",
	"Shadipur", "Moti Nagar", "Ramesh Nagar", "Green Park", "Hauz Khas",
	"Malviya Nagar", "Saket", "Qutub Minar", "Chhatarpur", "Mehrauli",
	"Nehru Place", "Kalkaji", "Govind Puri", "Okhla", "Jasola",
	"Sarita Vihar", "Badarpur", "Tughlakabad", "Lajpat Nagar", "Moolchand",
	"AIIMS", "INA Market", "Dilli Haat", "Jor Bagh", "Lodhi Colony",
	"Khan Market", "Mandi House", "Barakhamba Road", "Rajiv Chowk", "Patel Chowk",
	"Central Secretariat", "Udyog Bhawan", "Race Course", "Jangpura", "Ashram",
	"Kashmere Gate", "Civil Lines", "Vidhan Sabha", "Model Town", "Azadpur",
	"Adarsh Nagar", "Jahangirpuri", "Rohini Sector 18", "Pitampura", "Netaji Subhash Place",
	"Shalimar Bagh", "Punjabi Bagh", "Paschim Vihar", "Peera Garhi", "Mundka",
	"Anand Vihar", "Karkarduma", "Preet Vihar", "Laxmi Nagar", "Yamuna Bank",
	"Mayur Vihar", "New Ashok Nagar", "Vaishali", "Kaushambi", "Shahdara",
	"Welcome", "Seelampur", "Dilshad Garden", "Rithala", "Seemapuri",
	"Vasant Kunj", "Vasant Vihar", "Munirka", "RK Puram", "Safdarjung",
	"Chanakyapuri", "Dhaula Kuan", "Naraina", "Inderpuri", "Pusa Road",
	"Daryaganj", "Chandni Chowk", "Chawri Bazar", "Jama Masjid", "Red Fort",
	"ITO", "Pragati Maidan", "Sarai Kale Khan", "Nizamuddin", "Hazrat Nizamuddin",
}

// stopSuffixes vary the generated stop names so a corridor does not read as
// the same name repeated.
var stopSuffixes = []string{
	"Bus Stand", "Crossing", "Terminal", "Depot", "Chowk", "Market",
	"Metro Station", "Village", "Gate No 1", "Block C", "Main Road",
}

// stop is a generated bus stop. Stops are shared between routes that pass
// close to one another, which is what produces interchanges.
type stop struct {
	ID   string
	Name string
	Code string
	Pos  point
}

// route is a generated bus route and the path it follows.
type route struct {
	ID        string
	ShortName string
	LongName  string
	Color     string
	Shape     polyline
	StopIDs   []string
}

// network is a whole generated bus network.
type network struct {
	Routes []*route
	Stops  []*stop

	// byCell indexes stops by a coarse grid cell so routes running along the
	// same corridor share stops instead of stacking their own on top.
	byCell map[string]*stop
}

// cellKey buckets a position into roughly a 150m grid.
func cellKey(p point) string {
	return fmt.Sprintf("%.3f:%.3f", p.Lat, p.Lng)
}

// addStop returns the stop at p, creating one if no stop is already nearby.
func (n *network) addStop(p point, rnd *rand.Rand) *stop {
	key := cellKey(p)
	if s, ok := n.byCell[key]; ok {
		return s
	}

	name := fmt.Sprintf("%s %s",
		localities[rnd.Intn(len(localities))],
		stopSuffixes[rnd.Intn(len(stopSuffixes))])

	s := &stop{
		ID:   fmt.Sprintf("S%05d", len(n.Stops)+1),
		Name: name,
		Code: fmt.Sprintf("%05d", 10000+len(n.Stops)+1),
		Pos:  p,
	}
	n.Stops = append(n.Stops, s)
	n.byCell[key] = s
	return s
}

// buildCorridor traces a path from one point to another, bending it along the
// way so it does not read as a straight line.
func buildCorridor(from, to point, seed float64, steps int) polyline {
	dLat := to.Lat - from.Lat
	dLng := to.Lng - from.Lng

	// Unit vector perpendicular to the corridor, to offset points sideways.
	length := math.Hypot(dLat, dLng)
	if length == 0 {
		return polyline{from}
	}
	perpLat, perpLng := -dLng/length, dLat/length

	line := make(polyline, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Taper the bend to zero at both ends so corridors still meet the
		// terminals they are supposed to connect.
		amp := math.Sin(t*math.Pi) * length * 0.13
		off := wobble(t, seed) * amp
		line = append(line, point{
			Lat: from.Lat + dLat*t + perpLat*off,
			Lng: from.Lng + dLng*t + perpLng*off,
		})
	}
	return line
}

// buildRing traces a closed loop around the centre at a given radius.
func buildRing(radius, seed float64, steps int) polyline {
	line := make(polyline, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		angle := t * 2 * math.Pi
		r := radius * (1 + wobble(t, seed)*0.12)
		line = append(line, point{
			Lat: centreLat + r*math.Sin(angle),
			// Longitude degrees are shorter than latitude degrees at this
			// latitude, so widen the ring to keep it circular on screen.
			Lng: centreLng + r*math.Cos(angle)/math.Cos(centreLat*math.Pi/180),
		})
	}
	return line
}

// buildNetwork generates radial corridors, ring roads and cross-town routes,
// then hangs stops off them.
func buildNetwork(seed int64, radials, rings, crosstown int) *network {
	rnd := rand.New(rand.NewSource(seed))
	n := &network{byCell: map[string]*stop{}}

	add := func(shortName, longName string, shape polyline, stopEvery int) {
		r := &route{
			ID:        shortName,
			ShortName: shortName,
			LongName:  longName,
			Color:     routeColors[len(n.Routes)%len(routeColors)],
			Shape:     shape,
		}
		for i := 0; i < len(shape); i += stopEvery {
			r.StopIDs = append(r.StopIDs, n.addStop(shape[i], rnd).ID)
		}
		// A route needs at least two stops to produce a usable schedule.
		if len(r.StopIDs) < 2 {
			return
		}
		n.Routes = append(n.Routes, r)
	}

	// Radial corridors, running from the centre out to the city edge.
	for i := 0; i < radials; i++ {
		angle := float64(i) / float64(radials) * 2 * math.Pi
		reach := 0.11 + rnd.Float64()*0.10
		outer := point{
			Lat: centreLat + reach*math.Sin(angle),
			Lng: centreLng + reach*math.Cos(angle)/math.Cos(centreLat*math.Pi/180),
		}
		inner := point{
			Lat: centreLat + 0.012*math.Sin(angle),
			Lng: centreLng + 0.012*math.Cos(angle)/math.Cos(centreLat*math.Pi/180),
		}
		shape := buildCorridor(inner, outer, float64(i)*1.9, 44)
		terminus := localities[rnd.Intn(len(localities))]
		add(fmt.Sprintf("%d", 100+i*7),
			fmt.Sprintf("Connaught Place to %s", terminus), shape, 4)
	}

	// Ring roads.
	for i := 0; i < rings; i++ {
		radius := 0.045 + float64(i)*0.048
		shape := buildRing(radius, float64(i)*3.3+1.1, 96)
		add(fmt.Sprintf("%dR", 400+i*6),
			fmt.Sprintf("Ring Road Circular %d", i+1), shape, 6)
	}

	// Cross-town routes, which do not touch the centre at all.
	for i := 0; i < crosstown; i++ {
		a := rnd.Float64() * 2 * math.Pi
		b := a + math.Pi*0.55 + rnd.Float64()*math.Pi*0.6
		ra := 0.07 + rnd.Float64()*0.09
		rb := 0.07 + rnd.Float64()*0.09
		from := point{
			Lat: centreLat + ra*math.Sin(a),
			Lng: centreLng + ra*math.Cos(a)/math.Cos(centreLat*math.Pi/180),
		}
		to := point{
			Lat: centreLat + rb*math.Sin(b),
			Lng: centreLng + rb*math.Cos(b)/math.Cos(centreLat*math.Pi/180),
		}
		shape := buildCorridor(from, to, float64(i)*2.4+0.7, 52)
		add(fmt.Sprintf("%dX", 700+i*5),
			fmt.Sprintf("%s to %s",
				localities[rnd.Intn(len(localities))],
				localities[rnd.Intn(len(localities))]), shape, 5)
	}

	return n
}
