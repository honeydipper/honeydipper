# Setting up Keycloak for SAML Testing

Keycloak is running locally on **http://localhost:8280** with admin credentials:
- Username: `admin`
- Password: `admin`

## Step 1: Access Keycloak Admin Console

1. Open http://localhost:8280/admin in your browser
2. Login with admin credentials above

## Step 2: Create a SAML Client

1. In the left sidebar, go to **Clients** (or **Applications > Clients** in newer versions)
2. Click **Create client**
3. Fill in:
   - **Client ID**: `honeydipper` (or any name)
   - **Client Protocol**: `saml`
   - Click **Save**

4. In the **Settings** tab, configure:
   - **Master SAML Processing URL**: `http://localhost:9000/api/auth/saml/callback`
   - **Valid Redirect URIs**: `http://localhost:9000/api/auth/saml/callback`
   - Toggle **Sign Documents**: ON
   - Toggle **Sign Assertions**: ON
   - **Name ID Format**: `email` or `persistent` (if you see this field)
   - Click **Save**

5. In the **Keys** tab:
   - Copy the certificate (you'll use this if you enable response validation)

## Step 3: Get the IdP Metadata URL

In Keycloak, the SAML metadata is not shown in a UI field—it's auto-generated and available at a URL. For the **master** realm, it's:

```
http://localhost:8280/realms/master/protocol/saml/descriptor
```

You can verify it works by visiting that URL in your browser—it should return an XML file with the IdP metadata.

If you created a different realm, use:
```
http://localhost:8280/realms/{REALM_NAME}/protocol/saml/descriptor
```

Use this URL in your Honeydipper config's `idp_metadata_url` field.

## Step 4: Create Test Users

1. In the left sidebar, go to **Users** (or **Manage > Users**)
2. Click **Add user**
3. Fill in:
   - **Username**: `testuser`
   - **Email**: `testuser@example.com`
   - **First Name**: `Test`
   - **Last Name**: `User`
   - Click **Create**

4. In the **Credentials** tab:
   - Click **Set password**
   - Enter a password and toggle **Temporary** to OFF
   - Click **Set Password**

## Step 5: Test with Honeydipper

Update your `sample-auth-saml.yaml`:

```yaml
drivers:
  auth-saml:
    package: builtin
    type: auth-saml
    config:
      acs_url: "http://localhost:9000/api/auth/saml/callback"
      idp_metadata_url: "http://localhost:8280/realms/master/protocol/saml/descriptor"
      jwt_signing_key: "your-secret-key-for-signing-jwt"
      entity_id: "honeydipper"  # must match client ID in Keycloak
      token_expiration: 3600
```

Then:
1. Start Honeydipper: `go run ./cmd/honeydipper -c <your-config>`
2. Visit `http://localhost:9000/api/auth/saml/login`
3. You should be redirected to Keycloak login
4. Login with the test user credentials
5. Keycloak should redirect back to `/api/auth/saml/callback` with a SAMLResponse
6. Honeydipper should parse the response and create a session JWT

## Debugging

Check Keycloak logs:
```bash
podman logs -f <container_name_or_id>
```

Common issues:
- **ACS URL mismatch**: Make sure the `acs_url` in config exactly matches what's configured in Keycloak client
- **Entity ID mismatch**: The `entity_id` in config should match the **Client ID** in Keycloak
- **Metadata URL unreachable**: Keycloak must be accessible at the metadata URL from Honeydipper
- **SSL certificate validation**: In dev, crewjam/saml validates signatures; ensure IdP metadata is accessible
- **Name ID format**: Ensure the Name ID format in Keycloak matches what your SP expects (usually `email` or `persistent`)

## Stop Keycloak

When done testing:
```bash
podman stop <container_name_or_id>
```

Or just restart your system—the `--rm` flag will clean it up on stop.
