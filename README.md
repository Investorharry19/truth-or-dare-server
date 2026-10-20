# Truth or Dare Server

A Go backend server for a Truth or Dare game application. This service provides user authentication, payments and points management, real-time room and websocket support, and scheduled background maintenance.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Requirements](#requirements)
- [Environment Variables](#environment-variables)
- [Setup](#setup)
- [Database](#database)
- [Running](#running)
- [API Endpoints](#api-endpoints)
- [WebSocket](#websocket)
- [Scheduled Jobs](#scheduled-jobs)
- [Project Structure](#project-structure)
- [Notes](#notes)

## Features

- Email/password-based authentication
- Google sign-in support
- Refresh token flow
- Email verification and password reset
- User points and streak tracking
- Paystack payment initialization, verification, and webhook handling
- Room creation and in-memory websocket game sessions
- File uploads via Cloudinary
- Background cron job to reset free points daily

## Architecture

The server is built with Gin and PostgreSQL, using these main components:

- `main.go` - application entrypoint, HTTP route registration, middleware, cron startup
- `internal/auth` - authentication service and routes
- `internal/room` - room lifecycle, websocket handling, game state
- `internal/payments` - payment logic, Paystack integration, transfers
- `internal/user` - user repository operations
- `pkg/db` - PostgreSQL connection pool helper
- `pkg/jwt` - JWT generation and validation
- `cronjobs` - scheduled tasks and background maintenance

## Requirements

- Go 1.25.0
- PostgreSQL
- Paystack account for payment integration

## Environment Variables

Create a `.env` file or provide these variables in your environment:

- `PORT` - server port (default `8080`)
- `DATABASE_URL` - PostgreSQL connection string
- `PAYSTACK_SECRET_KEY` - Paystack secret API key
- `PAYSTACK_PUBLIC_KEY` - Paystack public key
- `JWT_SECRET` - JWT access token signing secret
- `JWT_REFRESH_SECRET` - JWT refresh token signing secret (if not set, `JWT_SECRET` is reused)

## Setup

1. Clone the repository.
2. Ensure PostgreSQL is running.
3. Create a `.env` file in the project root with the required variables.
4. Run:

```bash
cd server
go mod tidy
```

## Database

This service expects a PostgreSQL database with the following migration applied:

- `migrations/001_create_users.sql`

Example migration contents:

- `users` table with `id`, `username`, `email`, `password_hash`, `points`, `created_at`

If additional schema migrations exist, apply those as needed.

## Running

Start the server locally:

```bash
go run main.go
```

Or build and run:

```bash
go build -o truth-or-dare-server .
.
\truth-or-dare-server.exe
```

The server listens on the configured `PORT` and defaults to `8080`.

## API Documentation

When the server is running, open `http://localhost:8080/docs` for the interactive Swagger UI. The OpenAPI 3.0 specification is available at `http://localhost:8080/openapi.yaml`. Swagger UI loads its assets from a CDN, so an internet connection is needed to render the interactive page.

## API Endpoints

### Auth

- `POST /auth/register`
  - Body: `{ "username": "...", "email": "...", "fullName": "...", "password": "..." }`
- `POST /auth/login`
  - Body: `{ "email": "...", "password": "..." }`
- `POST /auth/google`
  - Body: `{ "idToken": "..." }`
- `POST /auth/refresh`
  - Body: `{ "refreshToken": "..." }`
- `POST /auth/forgot-password`
  - Body: `{ "email": "..." }`
- `POST /auth/reset-password`
  - Body: `{ "token": "...", "password": "..." }`
- `POST /auth/verify-email`
  - Body: `{ "token": "..." }`
- `GET /auth/verify-email?token=...`
- `POST /auth/resend-email-verification-token`
  - Body: `{ "email": "..." }`
- `POST /auth/logout`
  - Requires auth header
- `GET /auth/me`
  - Requires auth header
- `GET /auth/points`
  - Requires auth header
- `GET /auth/streak`
  - Requires auth header
- `POST /auth/streak`
  - Requires auth header

### Room

- `POST /room/create`
  - Requires auth header
- `POST /room/lobbies/:id/claim`
  - Requires auth header
  - Requires an active `/ws` connection; claims an ownerless generated lobby and joins the claimant as its first participant
  - JSON body requires `username`; optional `name` (up to 40 characters) and `max_players` (2-20) customize the lobby
- `GET /room/:id`
  - Requires auth header
- `POST /room/:id/upload`
  - Requires auth header

### Payments

- `POST /payment/initialize`
  - Body: `{ "points": 120 }`
  - Requires auth header
- `GET /payment/verify/:reference`
  - Requires auth header
- `GET /payment/callback`
  - Paystack callback route
- `POST /payment/webhook`
  - Paystack webhook route

### Points

- `POST /points/transfer`
  - Requires auth header
- `GET /points/transfers`
  - Requires auth header

### Health

- `GET /health`
  - Returns JSON status
- `GET /`
  - Welcome message and last update metadata

## WebSocket

The websocket endpoint is:

- `GET /ws`

Send a message with `type: "subscribe_lobbies"` to receive a `lobbies_snapshot` event. The server broadcasts `lobbies_updated` events with the current open-lobby list whenever lobbies are created, claimed, joined, filled, started, or removed. The server maintains a pool of five public, claimable lobbies and replenishes it when fewer than five public open lobbies are available.

The websocket connection uses query parameters for authentication:

- `token` - JWT access token
- `username` - user display name

The room subsystem supports:

- joining and leaving rooms
- private rooms with passcodes
- participant tracking and room updates
- inactivity cleanup for empty or inactive rooms
- Cloudinary upload support for room assets

## Scheduled Jobs

- `cronjobs.StartPointsResetCron` runs every 24 hours and resets free points for users if more than 24 hours have passed since the last reset.
- `room.StartInactivityCleanup` checks every second and applies a 5-minute inactivity timeout to claimed rooms: rooms with at least two players start automatically, while rooms with fewer than two players close. Unclaimed generated lobbies remain open.

## Project Structure

- `main.go` - app bootstrap and route setup
- `internal/auth` - auth handlers and service logic
- `internal/room` - room management, websocket handling, game state
- `internal/payments` - Paystack payment and point transfer logic
- `internal/user` - user repository access and persistence
- `pkg/db` - Postgres connection helper
- `pkg/jwt` - JWT helper functions
- `cronjobs` - background cron logic
- `migrations` - database migration SQL files

## Notes

- JWT secrets must be set before starting the server.
- `DATABASE_URL` is mandatory and used to establish the Postgres pool.
- The project currently uses an in-memory room registry (`map[string]*Room`) and is not horizontally scalable without additional shared state.
- Websocket reconnection is supported for users with the same JWT user ID.
- Most endpoints require `Authorization: Bearer <token>` headers via middleware.

---
