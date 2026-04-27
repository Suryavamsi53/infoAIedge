# 🚀 SVV Portfolio - Cloud Native & Production Ready

A premium, state-of-the-art portfolio platform featuring a Go backend, JWT-secured admin dashboard, and global visitor tracking. Optimized for containerized deployment with PostgreSQL.

## 🛠️ Key Features
- **Dynamic Glassmorphism UI**: High-end aesthetic with vibrant gradients and smooth interactions.
- **Interactive Admin Dashboard**: Manage contact transmissions in a Gmail-style split-pane interface.
- **Global Visitor Analysis**: Real-time visualization of transmissions on a global map using Leaflet.js.
- **Production-Grade Backend**: Go (Golang) server with PostgreSQL support and JWT-based authentication.
- **Containerized**: Fully Dockerized for seamless deployment to any cloud provider.

---

## 📦 Production Deployment Guide

### 1. Recommended Platforms
- **Railway.app** (Highest Recommendation)
- **Render.com**
- **Google Cloud Run**

### 2. Environment Variables
To run in production, ensure these variables are set in your hosting provider:
- `PORT`: 8001 (default)
- `JWT_SECRET`: A long, random string for token signing.
- `ADMIN_USER`: Your desired admin username.
- `ADMIN_PASS`: Your desired admin password.
- `DATABASE_URL`: Your PostgreSQL connection string (Ex: `postgres://user:pass@host:port/db`).
  - *If `DATABASE_URL` is omitted, the app will fall back to local `messages.json` storage (not recommended for production).*

### 3. Deploying with Docker
The included `Dockerfile` uses a multi-stage build to produce a minimal, high-performance image.

```bash
# Build the image
docker build -t svv-portfolio .

# Run locally
docker run -p 8001:8001 \
  -e JWT_SECRET=random_secret \
  -e DATABASE_URL=your_db_url \
  svv-portfolio
```

---

## 🎨 Technology Stack
- **Frontend**: HTML5, Vanilla CSS (Modern CSS variables), ES6+ JavaScript.
- **Backend**: Go (Golang) 1.22+
- **Persistence**: PostgreSQL (Primary) / JSON (Fallback)
- **Security**: JWT (JSON Web Tokens), Bcrypt Password Hashing.
- **Maps**: OpenStreetMap/Leaflet integration with Nominatim Geocoding.

## 🔒 Security Best Practices
- **Change Credentials**: Never deploy with default passwords. Set `ADMIN_USER` and `ADMIN_PASS` via environment variables.
- **HTTPS**: Ensure your hosting provider provides SSL (Railway and Render do this automatically).
- **Environment Secrets**: Never hardcode secrets in the source code.

---

## 👨‍💻 Local Development
1. Clone the repository.
2. Install dependencies: `go mod tidy`.
3. Run the server: `go run main.go`.
4. Access at `http://localhost:8001`.
