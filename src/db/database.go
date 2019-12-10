package db

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"math"
	"points"
	"strings"
)

var (
	db *sqlx.DB
)

func InitDB() {
	cfg := readConfig()

	source := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		cfg.Database.Username, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	database, err := sqlx.Open("mysql", source)
	if err != nil {
		log.Fatal(err)
	}

	db = database
}

// DBObjectById gets object from database.
func DBObjectById(id int64) (*DBObject, error) {
	var point = DBObject{}

	err := db.Get(&point, "select * from points where (active=true and id=?)", id)
	if err != nil {
		return nil, err
	}

	return &point, nil
}

func GetDBObjectsInRange(a, b points.Point) []DBObject {
	maxLat := math.Max(a.Lat, b.Lat)
	minLat := math.Min(a.Lat, b.Lat)
	maxLon := math.Max(a.Lon, b.Lon)
	minLon := math.Min(a.Lon, b.Lon)

	result := []DBObject{}
	query := fmt.Sprintf(
		"select * from points where (%f <= lat and lat <= %f and %f <= lon and lon <= %f)",
		minLat, maxLat, minLon, maxLon)

	err := db.Select(&result, query)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return result
}

func GetRoundDBRoute(start points.Point, radius int) (*DBRoute, error) {
	var route = DBRoute{}
	query := fmt.Sprintf(
		"select count(*) from routes where (start_lat=%f and start_lon=%f and radius=%f)",
		start.Lat, start.Lon, radius)
	err := db.Get(&route, query)
	if err != nil {
		if msg := err.Error(); strings.Contains(msg, "no rows in result") {
			return nil, nil
		}
		return nil, err
	}

	query = fmt.Sprintf(
		"update routes set count=count + 1 where (start_lat=%f and start_lon=%f and radius=%f)",
		start.Lat, start.Lon, radius)
	db.Exec(query)

	return &route, nil
}

func GetDirectDBRoute(a, b points.Point) (*DBRoute, error) {
	var route = DBRoute{}
	query := fmt.Sprintf(
		"select * from routes where (start_lat=%f and start_lon=%f and finish_lat=%f and finish_lon=%f)",
		a.Lat, a.Lon, b.Lat, b.Lon)

	err := db.Get(&route, query)
	if err != nil {
		if msg := err.Error(); strings.Contains(msg, "no rows in result") {
			return nil, nil
		}
		return nil, err
	}

	query = fmt.Sprintf(
		"update routes set count=count + 1 where (start_lat=%f and start_lon=%f and finish_lat=%f and finish_lon=%f)",
		a.Lat, a.Lon, b.Lat, b.Lon)
	db.Exec(query)

	return &route, nil
}

func DBRouteById(id int64) (*DBRoute, error) {
	var route = DBRoute{}

	err := db.Get(&route, "select * from routes where id=?", id)
	if err != nil {
		return nil, err
	}

	return &route, nil
}

func InsertDirectRoute(route DBRoute) int64 {
	query := fmt.Sprintf(
		"insert into routes (type, start_lat, start_lon, finish_lat, finish_lon, length, time, objects, points, name) values (%s, %f, %f, %f, %f, %f, %v, %s, %s, %s)",
		route.Type,
		route.Start_lat, route.Start_lon, route.Finish_lat, route.Finish_lon,
		route.Length, route.Time,
		route.Objects, route.Points, route.Name,
		)

	res, _ := db.Exec(query)
	id, _ := res.LastInsertId()
	return id
}

func InsertRoundRoute(route DBRoute) int64 {
	query := fmt.Sprintf(
		"insert into routes (type, start_lat, start_lon, radius, length, time, objects, points, name) values (%s, %f, %f, %f, %f, %f, %v, %s, %s, %s)",
		route.Type,
		route.Start_lat, route.Start_lon, route.Radius,
		route.Length, route.Time,
		route.Objects, route.Points, route.Name,
	)

	res, _ := db.Exec(query)
	id, _ := res.LastInsertId()
	return id
}