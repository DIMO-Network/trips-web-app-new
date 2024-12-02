package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
)

func (v *VehiclesController) HandleDecodeVIN(c *fiber.Ctx) error {
	// todo refactor this into method GetAccessJWT
	sessionCookie := c.Cookies("session_id")
	jwtToken, found := CacheInstance.Get(sessionCookie)
	if !found {
		return c.Render("session_expired", fiber.Map{})
	}

	accessToken, ok := jwtToken.(string)
	if !ok {
		return c.Render("session_expired", fiber.Map{})
	}

	type VINDecodeRequest struct {
		VIN string `json:"vin"`
	}
	var req VINDecodeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.VIN == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "VIN is required",
		})
	}
	log.Info().Msgf("Received request for VIN decode: %s", req.VIN)
	decodeVinResp, err := callExternalAPI(v.settings.DecodeVINEndpoint, req, accessToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decode VIN",
		})
	}
	decodedDefinitionID := gjson.GetBytes(decodeVinResp, "deviceDefinitionId").String()
	newDDTrxHash := gjson.GetBytes(decodeVinResp, "newTransactionHash").String()

	if decodedDefinitionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to decode VIN"})
	}

	return c.JSON(fiber.Map{
		"deviceDefinitionId": decodedDefinitionID,
		"newTransactionHash": newDDTrxHash,
	})
}

// Helper function to call an external REST API
func callExternalAPI(apiURL string, input any, bearerToken string) ([]byte, error) {
	// Convert the vehicle struct to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Make the POST request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer YOUR_API_TOKEN") // Replace with your actual token
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}
