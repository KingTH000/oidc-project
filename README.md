# Overview of OpenID Connect (OIDC)

OpenID Connect (OIDC) is a modern identity protocol that enables our applications to securely authenticate users. It is a thin identity layer built on top of the **OAuth 2.0** framework.





OIDC extends OAuth 2.0 by adding a few key components to provide a full authentication solution:

### ID Token

The **ID Token** is a **JSON Web Token (JWT)**—a digitally signed, self-contained JSON object. While the Access Token is for the API, the ID Token is for the **client application** (`app.go`).

It contains identity information about the user, known as **claims**. Key claims include:
* `sub` (Subject): A unique, stable identifier for the user (e.g., their user ID from our database).
* `iss` (Issuer): The URL of our OIDC provider that issued the token (e.g., `http://localhost:8080`).
* `aud` (Audience): The `client_id` of the application the token was intended for (e.g., `test-client`).
* `exp` (Expiration): The time when the token expires.

### Standardized Scopes

OIDC introduces a new, mandatory scope: `openid`. Requesting this scope is what signals that the client is initiating an OIDC flow and wants an ID Token. OIDC also standardizes scopes for common profile data:
* `profile`: Requests access to user's name, username, etc.
* `email`: Requests access to the user's `email` and `email_verified` claims.


### Why OIDC

* **Centralized Security**: Having only one service that handles passwords. This reduces the security attack surface. The other applications no longer need to store or hash credentials.
* **Standardization & Interoperability**: By building to the OIDC standard, any future application (web, mobile, or third-party) can integrate with the user system using any standard OIDC client library.
* **Stateless and Scalable API Security**: By using JWT access tokens (the "JWKS version"), the APIs are stateless. They don't need to share a database or session store with the identity provider. They can validate tokens locally, which is extremely fast and scales horizontally.


  
----



# Go OIDC Provider & Client Example

This project is a fully functional OpenID Connect (OIDC) provider and an example client application built with Go. It demonstrates a complete authentication and authorization flow, from user login with database integration to securing an API with JWTs.

## About The Project

This repository contains two main components:

  * **OIDC Provider (`oidc-server`)**: A robust OIDC provider built using the `luikyv/go-oidc` library. It handles user authentication, issues tokens, and provides a secure API.
  * **Client Application (`oidc-client-app`)**: A sample client application (Relying Party) built with the `zitadel/oidc` library. It shows how to integrate with the OIDC provider to log in users and access protected resources on their behalf.

The goal of this project is to serve as a practical, real-world example of implementing modern identity and security protocols in a Go application.

In the project, OIDC allows the `oidc-server` application to act as a central and secure Identity Provider. This lets our client applications (`oidc-client-app`) verify a user's identity and receive their profile information without ever having to handle a password, significantly improving our security and centralizing user management.

### Features

**Database Integration**: User credentials are securely stored in a MySQL database, with password hashing handled by `bcrypt`.

**User Consent Screen**: After logging in, users are presented with a consent screen detailing the specific permissions (scopes) the client application is requesting.

**API Protection**: Includes a protected API endpoint (`/api/posts`) that serves user-specific data from the database.

**Local JWT Validation (JWKS)**: Access tokens are issued as JWTs. The API validates them locally and efficiently by fetching the provider's public keys from the JWKS endpoint.

**Custom Scopes**: Implements custom scopes (e.g., `posts.read`) to manage granular access to API resources.

-----

### Technical Flow

1.  **Authorization Request**: The user clicks "Login" in the `app.go` client. The client redirects them to our OIDC provider's `/authorize` endpoint, requesting scopes like `openid profile posts.read`.
2.  **Authentication & Consent**:
    * It first renders the `login.html` template and verifies the user's credentials against our MySQL database.
    * After successful login, it renders the `consent.html` template, showing the user which permissions the client is requesting (e.g., "Read your blog posts").
3.  **Code Issuance**: When the user clicks "Allow," the provider redirects back to the client's registered `redirect_uri` with a one-time-use **authorization code**.
4.  **Token Exchange**: The client (`app.go`) sends this authorization code, along with its `client_id` and `client_secret`, to the provider's `/token` endpoint.
5.  **Token Response**: The provider's token endpoint validates the code and secret, then returns the final tokens:
    * **ID Token (JWT)**: Proves the user's identity to the client.
    * **Access Token (JWT)**: Grants permission for the client to access the API.
6.  **Client-Side Validation**: The `app.go` client validates the ID Token's signature using the provider's `/jwks` endpoint. It can now trust the user's identity and establish a local session.
7.  **API Access**: The client makes a request to our `/api/posts` endpoint, placing the **Access Token** in the `Authorization` header.
8.  **API-Side Validation**: Our `/api/posts` handler receives the request. It uses the verifier from the `coreos/go-oidc` library to perform local validation on the JWT Access Token. It checks the signature against the provider's `/jwks` keys, verifies the `aud` (audience) claim, and ensures the token has the required `posts.read` scope. If all checks pass, it serves the user's data.
<img width="438" height="275" alt="user login image" src="https://github.com/user-attachments/assets/fb53f4d8-e211-4f0a-ae44-041692188df2" />
<img width="438" height="275" alt="user consent image" src="https://github.com/user-attachments/assets/92446527-fc83-46b9-9095-3ec3b1fa070a" />



-----

## Getting Started

Follow these steps to get the project running on local machine.

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
    You should see log messages indicating that the server is connected to the database, running, and the JWT Verifier has been initialized.

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

