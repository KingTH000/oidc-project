# Go OIDC Provider & Client Example

This project is a fully functional OpenID Connect (OIDC) provider and an example client application built with Go. It demonstrates a complete authentication and authorization flow, from user login with database integration to securing an API with token introspection and custom scopes.

## About The Project

This repository contains two main components:

  * **OIDC Provider (`oidc-server`)**: A robust OIDC provider built using the `luikyv/go-oidc` library. It handles user authentication, issues tokens, and provides a secure API.
  * **Client Application (`oidc-client-app`)**: A sample client application (Relying Party) built with the `zitadel/oidc` library. It shows how to integrate with the OIDC provider to log in users and access protected resources on their behalf.

The goal of this project is to serve as a practical, real-world example of implementing modern identity and security protocols in a Go application.

### Features

  * ✅ **Database Integration**: User credentials are securely stored in a MySQL database, with password hashing handled by `bcrypt`.
  * ✅ **User Consent Screen**: After logging in, users are presented with a consent screen detailing the specific permissions (scopes) the client application is requesting.
  * ✅ **API Protection**: Includes a protected API endpoint (`/api/posts`) that serves user-specific data from the database.
  * ✅ **Token Introspection**: The API is secured using the standard Token Introspection endpoint (`/introspect`), allowing the API to validate access tokens in a decoupled manner.
  * ✅ **Custom Scopes**: Implements custom scopes (e.g., `posts.read`) to manage granular access to API resources.

-----

## Getting Started

Follow these steps to get the project running on your local machine.

### Prerequisites

  * Go 1.18 or newer
  * MySQL server

### 1\. Database Setup

1.  Connect to your local MySQL server.
2.  Create a database. The application is configured to use a database named `blog`.
3.  Import the schema and sample data by running the `blog.sql` file provided in the repository. This will create the `users` and `posts` tables and insert a few sample users.

### 2\. Run the OIDC Provider

The provider runs on port `8080`.

1.  Navigate to the provider directory:
    ```sh
    cd oidc/oidc-server
    ```
2.  Install dependencies:
    ```sh
    go mod tidy
    ```
3.  Run the server:
    ```sh
    go run main.go
    ```
    You should see log messages indicating that the server is connected to the database and running.

### 3\. Run the Client Application

The client app runs on port `3000`.

1.  Open a **new terminal window**.
2.  Navigate to the client directory:
    ```sh
    cd oidc/oidc-client-app
    ```
3.  Set the required environment variables. These tell the client how to connect to the provider.
    ```sh
    export ISSUER="http://localhost:8080"
    export CLIENT_ID="test-client"
    export CLIENT_SECRET="client-secret"
    export PORT="3000"
    export SCOPES="openid profile email offline_access posts.read"
    ```
4.  Run the client application:
    ```sh
    go run app.go
    ```

### 4\. Test the Flow

1.  Open your web browser and go to **`http://localhost:3000/login`**.
2.  You will be redirected to the OIDC provider's login page. Use one of the sample users from the database (e.g., username `alice`, password `password123` or `bob`/`password123`).
3.  After logging in, you will be shown the consent screen. Click **Allow**.
4.  You will be redirected back to the client application, which will display your ID Token claims, the UserInfo response, and the list of posts fetched from the secure `/api/posts` endpoint.
