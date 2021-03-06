package routes

import (
	"db"
	"encoding/json"
	"errors"
	"filters"
	"fmt"
	"math"
	"objects"
	"osrm"
	"points"
	"routes/types"
)

type RouteType string

const (
	Direct RouteType = "direct"
	Round  RouteType = "round"
)

type Route struct {
	Objects []objects.Object `json:"objects"`
	Points  []points.Point   `json:"points"`
	Id      int64            `json:"id"`
	Length  float64          `json:"length"` //meters
	Time    int              `json:"time"`   //seconds
	Name    string           `json:"name"`
	Type    string           `json:"type"`
	radius  int
	filters int
}

func RouteByDBRoute(r *db.DBRoute) *Route {
	route := Route{
		Objects: []objects.Object{},
		Points:  []points.Point{},
		Id:      r.Id,
		Length:  r.Length,
		Time:    r.Time,
		Name:    r.Name,
		Type:    r.Type,
		radius:  r.Radius,
		filters: r.Filters,
	}

	err := json.Unmarshal([]byte(r.Points), &route.Points)
	if err != nil {
		route.Points = []points.Point{}
	}

	// todo: load data from database
	err = json.Unmarshal([]byte(r.Objects), &route.Objects)
	if err != nil {
		route.Objects = []objects.Object{}
	}

	return &route
}

func nameByRoute(route *Route) string {
	// хотим генерить что-то умное и триггерное
	// а пока по номеру
	return fmt.Sprintf("Route %v", route.Id)
}

func GetRoute(routeType string, points []points.Point, filters []string, radius int) (*Route, string) {
	var route *Route

	switch routeType {
	case string(Direct):
		route, _ = directRoute(points[0], points[1], filters)
	case string(Round):
		route, _ = roundRoute(points[0], radius, filters)
	default:
		return nil, "unsupported route type"
	}

	return route, ""
}

func directRoute(a, b points.Point, filters filters.StringFilter) (*Route, error) {
	route, err := getDirectRoute(a, b, filters)
	if route != nil {
		db.UpdateRouteCounter(route.Id)
		return route, err
	}

	allObjects := objects.GetAllObjectInRange(a, b, filters)
	pathObjects := routes.ABRoute{Start: a, Finish: b}.Build(allObjects)
	routeMainPoints := []points.Point{a}
	routeMainPoints = append(routeMainPoints, objects.PointsByObjects(pathObjects)...)
	routeMainPoints = append(routeMainPoints, b)

	route, err = getRouteByPoints(routeMainPoints)
	if err != nil {
		return nil, err
	}
	route.Objects = pathObjects
	route.Type = string(Direct)
	route.Id, err = saveInDB(route, filters.Int())
	if err != nil {
		return nil, err
	}
	route.Name = nameByRoute(route)
	db.UpdateRouteName(route.Id, route.Name)
	return route, err
}

func roundRoute(start points.Point, radius int, filters filters.StringFilter) (*Route, error) {
	route, err := getRoundRoute(start, radius, filters)
	if route != nil {
		db.UpdateRouteCounter(route.Id)
		return route, err
	}

	a := points.Point{
		Lat: start.Lat - routes.MetersToLat(float64(radius)),
		Lon: start.Lon - routes.MetersToLon(start, float64(radius)),
	}
	b := points.Point{
		Lat: start.Lat + routes.MetersToLat(float64(radius)),
		Lon: start.Lon + routes.MetersToLon(start, float64(radius)),
	}
	allObjects := objects.RandomObjectsInRange(a, b, 100, filters)
	pathObjects := routes.RoundRoute{Center: start, Radius: radius}.Build(allObjects)
	routeMainPoints := append([]points.Point{start}, objects.PointsByObjects(pathObjects)...)
	routeMainPoints = append(routeMainPoints, start)

	route, err = getRouteByPoints(routeMainPoints)
	if err != nil {
		return nil, err
	}
	route.Objects = pathObjects
	route.Type = string(Round)
	route.radius = radius
	route.Id, err = saveInDB(route, filters.Int())
	if err != nil {
		return nil, err
	}
	route.Name = nameByRoute(route)
	db.UpdateRouteName(route.Id, route.Name)
	return route, err
}

func roundCoordinates(coordinate float64) float64 {
	return math.Round(coordinate*1000) / 1000
}

func saveInDB(route *Route, filters int) (int64, error) {
	dbroute := db.DBRoute{
		Start_lat:  roundCoordinates(route.Points[0].Lat),
		Start_lon:  roundCoordinates(route.Points[0].Lon),
		Finish_lat: roundCoordinates(route.Points[len(route.Points)-1].Lat),
		Finish_lon: roundCoordinates(route.Points[len(route.Points)-1].Lon),
		Length:     route.Length,
		Time:       route.Time,
		Name:       route.Name,
		Filters:    filters,
	}
	var objectsIds []int64
	for i := range route.Objects {
		objectsIds = append(objectsIds, route.Objects[i].Id)
	}

	objectsJSON, _ := json.Marshal(objectsIds)
	dbroute.Objects = string(objectsJSON)
	pointsJSON, _ := json.Marshal(route.Points)
	dbroute.Points = string(pointsJSON)

	if route.Type == string(Direct) {
		dbroute.Type = string(Direct)
		return db.InsertDirectRoute(dbroute)
	} else {
		dbroute.Type = string(Round)
		dbroute.Radius = route.radius
		return db.InsertRoundRoute(dbroute)
	}
}

func getTimeByDistance(dist float64) int {
	speed := float64(25) / 18 // 5 km/h ~ 1.4 m/s
	return int(math.Round(dist / speed))
}

func getRouteByPoints(pathPoints []points.Point) (*Route, error) {
	osrmPath, err := osrm.GetOSRMByPoints(pathPoints)
	var route *Route
	if err == nil {
		route, err = routeByOSRMResponse(osrmPath)
	}

	if err != nil {
		return getRouteByPointsParts(pathPoints)
	}
	return route, nil
}

func getRouteByPointsParts(pathPoints []points.Point) (*Route, error) {
	route := Route{
		Points: []points.Point{},
		Length: 0,
		Time:   0,
	}
	for i := 1; i < len(pathPoints); i++ {
		// todo: make requests in parallel?
		osrmRoutePart, err := osrm.GetOSRMByPoints([]points.Point{pathPoints[i-1], pathPoints[i]})
		var routePart *Route
		if err == nil {
			routePart, err = routeByOSRMResponse(osrmRoutePart)
		}
		if err != nil {
			route.Points = append(route.Points, pathPoints[i-1], pathPoints[i])
			dist := routes.GetMetersDistanceByPoints(pathPoints[i-1], pathPoints[i])
			route.Length += dist
			route.Time += getTimeByDistance(dist)
		} else {
			route.Length += routePart.Length
			route.Time += routePart.Time
			route.Points = append(route.Points, routePart.Points...)
		}
	}
	return &route, nil
}

func RouteById(id int64) (*Route, error) {
	dbroute, err := db.DBRouteById(id)
	if err != nil {
		return nil, err
	}
	return RouteByDBRoute(dbroute), nil
}

func getDirectRoute(a, b points.Point, filters filters.StringFilter) (*Route, error) {
	a.Lat = roundCoordinates(a.Lat)
	a.Lon = roundCoordinates(a.Lon)
	b.Lat = roundCoordinates(b.Lat)
	b.Lon = roundCoordinates(b.Lon)
	dbroute, err := db.GetDirectDBRoute(a, b, filters.Int())
	if err != nil {
		return nil, err
	}
	return RouteByDBRoute(dbroute), nil
}

func getRoundRoute(start points.Point, radius int, filters filters.StringFilter) (*Route, error) {
	start.Lat = roundCoordinates(start.Lat)
	start.Lon = roundCoordinates(start.Lon)
	dbroute, err := db.GetRoundDBRoute(start, radius, filters.Int())
	if err != nil {
		return nil, err
	}
	return RouteByDBRoute(dbroute), nil
}

func routeByOSRMResponse(resp osrm.Response) (*Route, error) {
	if resp.Code != "Ok" {
		return nil, errors.New("bad OSRM")
	}
	osrmRoute := resp.Routes[0]
	route := Route{
		Points: []points.Point{},
		Length: osrmRoute.Distance,
		Time:   int(math.Round(osrmRoute.Duration)),
	}

	for _, g := range osrmRoute.Geometry.Coordinates {
		route.Points = append(route.Points, points.Point{g[1], g[0]})
	}
	return &route, nil
}

func RemovePoint(routeId, objectId int64) (*Route, error) {
	route, err := RouteById(routeId)
	if err != nil {
		return nil, err
	}

	if route.Type == string(Round) {
		return removePointFromRoundRoute(route, objectId)
	} else {
		return removePointFromDirectRoute(route, objectId)
	}
}

func searchInObjectSlice(objectId int64, slice []objects.Object) int {
	idInSlice := -1
	for i := 0; i < len(slice); i++ {
		if slice[i].Id == objectId {
			idInSlice = i
			break
		}
	}
	return idInSlice
}

func removePointFromRoundRoute(route *Route, objectId int64) (*Route, error) {
	idInSlice := searchInObjectSlice(objectId, route.Objects)
	if idInSlice == -1 {
		return nil, errors.New("no object with given id in the route")
	}
	routeObjects := append(route.Objects[:idInSlice], route.Objects[idInSlice+1:]...) //удаляем
	routeMainPoints := append([]points.Point{route.Points[0]}, objects.PointsByObjects(routeObjects)...)
	routeMainPoints = append(routeMainPoints, route.Points[0])

	radius := route.radius
	routeFilters := route.filters

	route, err := getRouteByPoints(routeMainPoints)
	if err != nil {
		return nil, err
	}

	route.Objects = routeObjects
	route.Type = string(Round)
	route.radius = radius
	route.Id, err = saveInDB(route, routeFilters)
	if err != nil {
		return nil, err
	}
	route.Name = nameByRoute(route)
	db.UpdateRouteName(route.Id, route.Name)

	return route, err
}

func removePointFromDirectRoute(route *Route, objectId int64) (*Route, error) {
	idInSlice := searchInObjectSlice(objectId, route.Objects)
	if idInSlice == -1 {
		return nil, errors.New("no object with given id in the route")
	}
	routeObjects := append(route.Objects[:idInSlice], route.Objects[idInSlice+1:]...) //удаляем
	routeMainPoints := append([]points.Point{route.Points[0]}, objects.PointsByObjects(routeObjects)...)
	routeMainPoints = append(routeMainPoints, route.Points[len(route.Points)-1])

	routeFilters := route.filters

	route, err := getRouteByPoints(routeMainPoints)
	if err != nil {
		return nil, err
	}

	route.Objects = routeObjects
	route.Type = string(Direct)
	route.Id, err = saveInDB(route, routeFilters)
	if err != nil {
		return nil, err
	}
	route.Name = nameByRoute(route)
	db.UpdateRouteName(route.Id, route.Name)

	return route, err
}
