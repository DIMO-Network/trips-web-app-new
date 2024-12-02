package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/DIMO-Network/shared"
	"github.com/dimo-network/trips-web-app/api/internal/config"
	"github.com/dimo-network/trips-web-app/api/internal/controllers"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/template/handlebars/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func ErrorHandler(ctx *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
		message = e.Message
	}

	log.Error().Err(err).Int("code", code).Str("path", ctx.Path()).Msg("Error occurred")

	return ctx.Status(code).JSON(fiber.Map{
		"error":   true,
		"message": message,
	})
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	fmt.Print("Server is starting...")

	settings, err := shared.LoadConfig[config.Settings]("settings.prod.yaml")
	if err != nil {
		log.Fatal().Err(err).Msg("could not load settings")
	}

	level, err := zerolog.ParseLevel(settings.LogLevel)
	if err != nil {
		log.Fatal().Err(err).Msgf("could not parse LOG_LEVEL: %s", settings.LogLevel)
	}
	zerolog.SetGlobalLevel(level)

	engine := handlebars.New("./views", ".hbs")

	ac := controllers.NewAccountController(settings)
	vc := controllers.NewVehiclesController(settings)
	tc := controllers.NewTripsController(settings)
	st := controllers.NewStreamrController(settings)

	app := fiber.New(fiber.Config{
		ErrorHandler:   ErrorHandler,
		Views:          engine,
		ReadBufferSize: 16000,
	})
	app.Use(cors.New())

	// View routes public
	app.Get("/login-jwt", ac.LoginWithJWT)
	app.Post("/login-jwt", ac.PostLoginWithJWT)

	// View routes (protected)
	app.Get("/account", controllers.AuthMiddleware(), ac.MyAccount)
	app.Get("/vehicles/me", controllers.AuthMiddleware(), vc.HandleGetVehicles)
	app.Get("/vehicles/new", controllers.AuthMiddleware(), vc.HandleNewVehicle)
	app.Get("/vehicles/:tokenid/signals", controllers.AuthMiddleware(), vc.HandleVehicleSignals)
	app.Get("/vehicles/:tokenid/history", controllers.AuthMiddleware(), vc.HandleGetHistoricalData)

	app.Get("/vehicles/:tokenid/trips", controllers.AuthMiddleware(), tc.HandleTripsList)
	app.Get("/give-feedback", controllers.AuthMiddleware(), controllers.HandleGiveFeedback(&settings))
	app.Get("/streamr", controllers.AuthMiddleware(), st.GetStreamr)

	// API routes called via Javascript fetch
	app.Get("/api/trip/:tripID", controllers.AuthMiddleware(), func(c *fiber.Ctx) error {
		tripID := c.Params("tripID")
		startTime := c.Query("start")
		endTime := c.Query("end")

		log.Info().Msgf("Received request for tripID: %s, startTime: %s, endTime: %s", tripID, startTime, endTime)

		// Retrieve the estimated start location from the query string
		var estimatedStart *controllers.LatLon
		if estimatedStartStr := c.Query("estimatedStart"); estimatedStartStr != "" {
			log.Info().Msgf("Received estimatedStart: %s", estimatedStartStr)
			if err := json.Unmarshal([]byte(estimatedStartStr), &estimatedStart); err != nil {
				log.Error().Err(err).Msg("Invalid estimated start location")
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid estimated start location"})
			}
		} else {
			log.Info().Msg("No estimatedStart received")
		}

		return controllers.HandleMapDataForTrip(c, &settings, tripID, startTime, endTime, estimatedStart)
	})
	app.Post("/api/decode-vin", controllers.AuthMiddleware(), vc.HandleDecodeVIN)

	// Public Routes
	app.Post("/auth/web3/generate_challenge", func(c *fiber.Ctx) error {
		return controllers.HandleGenerateChallenge(c, &settings)
	})
	app.Post("/auth/web3/submit_challenge", func(c *fiber.Ctx) error {
		return controllers.HandleSubmitChallenge(c, &settings)
	})

	app.Post("/api/generate-token/:tokenID", controllers.AuthMiddleware(), func(c *fiber.Ctx) error {
		tokenID, err := strconv.ParseInt(c.Params("tokenID"), 10, 64)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid token ID"})
		}

		token, err := controllers.RequestPriviledgeToken(c, &settings, tokenID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate privilege token", "details": err.Error()})
		}

		return c.JSON(fiber.Map{"token": *token})
	})

	// to hold any static css or js
	app.Static("/static", "./static", fiber.Static{
		Compress:      true,
		Download:      false,
		Index:         "",
		CacheDuration: 0,
		MaxAge:        0,
	})

	// used load react compiled login app
	app.Get("/", loadStaticIndex)

	// host the compiled frontend for the web3 login, which should be built to the dist folder
	staticConfig := fiber.Static{
		Compress: true,
		MaxAge:   0,
		Index:    "index.html",
	}
	app.Static("/", "./dist", staticConfig)

	app.Get("/health", healthCheck)

	log.Info().Msgf("Starting server on port %s", settings.Port)
	if err := app.Listen(":" + settings.Port); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}
}

func healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"code":    200,
		"message": "server is up",
	})
}

func loadStaticIndex(ctx *fiber.Ctx) error {
	dat, err := os.ReadFile("dist/index.html")
	if err != nil {
		return err
	}
	ctx.Set("Content-Type", "text/html; charset=utf-8")
	return ctx.Status(fiber.StatusOK).Send(dat)
}
