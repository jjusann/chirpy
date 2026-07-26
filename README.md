Chirpy API
A social media platform API (similar to Twitter) built with Go, featuring user authentication, chirp management, and subscription payments via webhooks.

🚀 Features
User Management – Create, update, and authenticate users with email/password

JWT Authentication – Access tokens (1‑hour expiry) and refresh tokens (60‑day expiry)

Password Hashing – Secure password storage using Argon2id

Chirp Operations – Create, read, delete, and filter chirps (140‑character limit)

Profanity Filtering – Automatically replaces profane words with ****

Chirpy Red Membership – Subscription-based premium membership via Polka webhooks

Admin Endpoints – Metrics dashboard and database reset (development only)

Static File Serving – Serves HTML, CSS, and images from the /app/ path

PostgreSQL Database – Relational database with UUID primary keys and timestamps

SQLC Code Generation – Type-safe SQL queries

📦 Tech Stack
Technology	Purpose
Go	Backend language
PostgreSQL	Relational database
SQLC	Type-safe SQL query generation
Goose	Database migrations
JWT	Authentication tokens
Argon2id	Password hashing
net/http	HTTP server (standard library)
🛠️ Setup & Installation
Prerequisites
Go 1.23+

PostgreSQL 13+

goose (migration tool)

sqlc (code generator)

1. Clone the repository
bash
git clone https://github.com/jjusann/chirpy.git
cd chirpy
2. Install dependencies
bash
go mod download
3. Set up environment variables
Create a .env file in the root directory:

env
DB_URL="postgres://postgres:postgres@localhost:5432/chirpy?sslmode=disable"
PLATFORM="dev"
JWT_SECRET="your_jwt_secret_here"
POLKA_KEY="your_polka_api_key_here"
4. Create the database
bash
createdb -U postgres chirpy
5. Run migrations
bash
goose -dir ./sql/schema postgres "postgres://postgres:postgres@localhost:5432/chirpy?sslmode=disable" up
6. Generate SQLC code
bash
sqlc generate
7. Build and run the server
bash
go build -o chirpy .
./chirpy
The server will start at http://localhost:8080.

📂 Project Structure
text
chirpy/
├── internal/
│   ├── auth/           # Authentication utilities (JWT, password hashing, tokens)
│   └── database/       # SQLC-generated database code
├── sql/
│   ├── schema/         # Goose migrations
│   └── queries/        # SQLC query files
├── main.go             # Application entry point
├── go.mod
├── go.sum
├── .env
└── README.md
🔗 API Endpoints
Health & Metrics
Method	Endpoint	Description
GET	/api/healthz	Readiness check
GET	/admin/metrics	Admin dashboard with hit count
POST	/admin/reset	Reset database (dev only)
User Management
Method	Endpoint	Description
POST	/api/users	Create a new user
PUT	/api/users	Update user email/password (authenticated)
POST	/api/login	Authenticate and receive tokens
POST /api/users – Request body:

json
{
  "email": "user@example.com",
  "password": "secure_password"
}
POST /api/login – Request body:

json
{
  "email": "user@example.com",
  "password": "secure_password"
}
Response:

json
{
  "id": "uuid",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "email": "user@example.com",
  "is_chirpy_red": false,
  "token": "jwt_token",
  "refresh_token": "refresh_token"
}
Chirp Management
Method	Endpoint	Description
POST	/api/chirps	Create a chirp (authenticated)
GET	/api/chirps	Get all chirps (optional ?author_id=uuid)
GET	/api/chirps/{chirpID}	Get a single chirp
DELETE	/api/chirps/{chirpID}	Delete a chirp (author only)
POST /api/chirps – Request body:

json
{
  "body": "This is my chirp!"
}
GET /api/chirps?author_id={uuid} – Filter chirps by author.

Token Management
Method	Endpoint	Description
POST	/api/refresh	Refresh access token using refresh token
POST	/api/revoke	Revoke refresh token
Webhooks (Polka)
Method	Endpoint	Description
POST	/api/polka/webhooks	Polka payment webhook (requires API key)
POST /api/polka/webhooks – Request body:

json
{
  "event": "user.upgraded",
  "data": {
    "user_id": "uuid"
  }
}
Static Files
Endpoint	Description
/app/	Serves static files from the root directory
🔐 Authentication
Access Tokens (JWT)
Pass in the Authorization header:

text
Authorization: Bearer <access_token>
Expires in 1 hour.

Refresh Tokens
Used to obtain a new access token.

Pass in the Authorization header for /api/refresh and /api/revoke:

text
Authorization: Bearer <refresh_token>
Expires in 60 days.

Can be revoked.

API Keys (Polka Webhook)
Pass in the Authorization header:

text
Authorization: ApiKey <polka_key>
🗃️ Database Schema
Users Table
Column	Type	Description
id	UUID	Primary key
created_at	TIMESTAMP	Creation timestamp
updated_at	TIMESTAMP	Last update timestamp
email	TEXT	User email (unique)
hashed_password	TEXT	Argon2id hashed password
is_chirpy_red	BOOLEAN	Premium membership status
Chirps Table
Column	Type	Description
id	UUID	Primary key
created_at	TIMESTAMP	Creation timestamp
updated_at	TIMESTAMP	Last update timestamp
body	TEXT	Chirp content (max 140 chars)
user_id	UUID	Foreign key to users(id)
Refresh Tokens Table
Column	Type	Description
token	TEXT	Primary key
created_at	TIMESTAMP	Creation timestamp
updated_at	TIMESTAMP	Last update timestamp
user_id	UUID	Foreign key to users(id)
expires_at	TIMESTAMP	Expiration timestamp
revoked_at	TIMESTAMP	Revocation timestamp (nullable)
🧪 Testing
Manual Testing with curl
bash
# Create a user
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}'

# Login
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}'

# Create a chirp (use the JWT token from login)
curl -X POST http://localhost:8080/api/chirps \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"body":"Hello, Chirpy!"}'

# Get all chirps
curl http://localhost:8080/api/chirps

# Filter chirps by author
curl "http://localhost:8080/api/chirps?author_id=<user_id>"
Running Tests
bash
go test ./...