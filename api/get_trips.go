package main

import (
	"encoding/json"
	"fmt"
	"github.com/dimo-network/trips-web-app/api/internal/config"
	"github.com/gofiber/fiber/v2"
	geojson "github.com/paulmach/go.geojson"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"net/url"
	"sort"
)

type Trip struct {
	ID    string    `json:"id"`
	Start TimeEntry `json:"start"`
	End   TimeEntry `json:"end"`
}

type TimeEntry struct {
	Time string `json:"time"`
}

type TripsResponse struct {
	Trips []Trip `json:"trips"`
}

var tripIDToTokenIDMap = make(map[string]int64)

type LocationData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func queryTripsAPI(tokenID int64, settings *config.Settings, c *fiber.Ctx) ([]Trip, error) {

	var tripsResponse TripsResponse

	sessionCookie := c.Cookies("session_id")
	privilegeTokenKey := "privilegeToken_" + sessionCookie

	// Retrieve the privilege token from the cache
	token, found := CacheInstance.Get(privilegeTokenKey)

	if !found {
		return nil, errors.New("privilege token not found in cache")
	}

	url := fmt.Sprintf("%s/vehicle/%d/trips", settings.TripsAPIBaseURL, tokenID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.(string))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&tripsResponse); err != nil {
		return nil, err
	}

	// Log each trip ID
	for _, trip := range tripsResponse.Trips {
		tripIDToTokenIDMap[trip.ID] = tokenID
		log.Info().Msgf("Trip ID: %s", trip.ID)
	}

	return tripsResponse.Trips, nil
}

func queryDeviceDataHistory(tokenID int64, startTime string, endTime string, settings *config.Settings, c *fiber.Ctx) ([]LocationData, error) {

	sessionCookie := c.Cookies("session_id")
	privilegeTokenKey := "privilegeToken_" + sessionCookie

	// Retrieve the privilege token from the cache
	token, found := CacheInstance.Get(privilegeTokenKey)

	if !found {
		log.Info().Msgf("priv token not found in cache")
		return nil, errors.New("privilege token not found in cache")
	}

	ddUrl := fmt.Sprintf("%s/vehicle/%d/history?startDate=%s&endDate=%s", settings.DeviceDataAPIBaseURL, tokenID, url.QueryEscape(startTime), url.QueryEscape(endTime))

	req, err := http.NewRequest("GET", ddUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.(string))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the raw response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Dynamically parse the JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}

	// Extract the hits array
	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})

	// Sort the hits based on the timestamp
	sort.SliceStable(hits, func(i, j int) bool {
		iTimestamp := hits[i].(map[string]interface{})["_source"].(map[string]interface{})["data"].(map[string]interface{})["timestamp"].(string)
		jTimestamp := hits[j].(map[string]interface{})["_source"].(map[string]interface{})["data"].(map[string]interface{})["timestamp"].(string)
		return iTimestamp < jTimestamp
	})

	// Convert sorted hits to LocationData
	locations := extractLocationData(hits)

	return locations, nil
}

func handleMapDataForTrip(c *fiber.Ctx, settings *config.Settings, tripID, startTime, endTime string) error {
	tokenID, exists := tripIDToTokenIDMap[tripID]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Trip not found")
	}

	log.Info().Msgf("HandleMapDataForTrip: TripID: %s, StartTime: %s, EndTime: %s, TokenID: %d", tripID, startTime, endTime, tokenID)

	// Fetch historical data for the specific trip
	locations, err := queryDeviceDataHistory(tokenID, startTime, endTime, settings, c)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch historical data: " + err.Error()})
	}

	// Convert the historical data to GeoJSON
	geoJSON := convertToGeoJSON(locations, tripID, startTime, endTime)

	geoJSONData, err := json.Marshal(geoJSON)
	if err != nil {
		log.Error().Msgf("Error with GeoJSON: %v", err)
	} else {
		log.Info().Msgf("GeoJSON data: %s", string(geoJSONData))
	}
	return c.JSON(geoJSON)
}

func extractLocationData(hits []interface{}) []LocationData {
	var locations []LocationData
	for _, hit := range hits {
		hitMap := hit.(map[string]interface{})
		data := hitMap["_source"].(map[string]interface{})["data"].(map[string]interface{})
		locData := LocationData{
			Latitude:  data["latitude"].(float64),
			Longitude: data["longitude"].(float64),
		}
		locations = append(locations, locData)
	}
	return locations
}

func convertToGeoJSON(locations []LocationData, tripID string, tripStart string, tripEnd string) *geojson.FeatureCollection {
	coords := make([][]float64, 0, len(locations))

	for _, loc := range locations {
		// Append each location as a coordinate pair in the coords slice
		coords = append(coords, []float64{loc.Longitude, loc.Latitude})
	}

	feature := geojson.NewLineStringFeature(coords)

	feature.Properties = map[string]interface{}{
		"type":         "LineString",
		"trip_id":      tripID,
		"trip_start":   tripStart,
		"trip_end":     tripEnd,
		"privacy_zone": 1,
		"color":        "black",
		"point-color":  "black",
	}

	// Create a feature collection and add the LineString feature to it
	fc := geojson.NewFeatureCollection()
	fc.AddFeature(feature)

	return fc
}
