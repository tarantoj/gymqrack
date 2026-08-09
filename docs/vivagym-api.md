# VivaGym App API notes

Decompiled from `apk/com.myvitale.vivagym.group.xapk` (VivaGym Group app,
package `com.myvitale.vivagym.group`) with jadx 1.5.6.

Base URL for all endpoints: `https://vivagym.myvitale.com/`

## Gym entry QR code

The QR code displayed in the app is **not generated locally** from member data.
The app calls an authenticated endpoint that returns a string payload, and that
string is simply rendered as a QR bitmap (512x512) client-side using the ZXing
library. Any validity/expiry window is enforced server-side inside the payload.

### Endpoint

`Api.java:215` (interface `com.myvitale.api.Api`):

```java
@GET("api/v2.0/exerp/qr")
Call<String> getQR();
```

- Method: `GET`
- Path: `api/v2.0/exerp/qr`
- Auth: `Authorization: Bearer <access_token>` (required)
- Response: HTTP 200 with a plain string body (the QR payload)

`GetQrInteractorImp.java:57` executes the call; on 200 the raw body string is
passed to `QrPresenterImpl.generateQR()` (`QrPresenterImpl.java:77`), which
encodes it with ZXing `QRCodeWriter` at 512x512 and shows it in the UI.

> Note: the `exerp` path indicates this talks to the **Exerp** gym-management
> platform (the same system used by VivaGym's access control / turnstiles).

## Authentication

Two-stage OAuth2 flow used by the current login code path
(`com.vitale.login.data.LoginRepositoryImpl`, `com.vitale.coredata`).

### Stage 1 — anonymous client credentials (gets a "temp" token)

`ApiHelperImpl.authenticate()` -> `ApiService.authentication()`
(`com.vitale.coredata.data.api.ApiService`):

```java
@POST("oauth/v2/token")
Object authentication(@Body OauthModel oauthModel, Continuation<...>);
```

The body is `OauthModel(grant_type, client_id, client_secret)`:

- `grant_type` = `client_credentials`
- `client_id` = `4_43uq8rgou3y88ckkk0sgg8c408w4gwsssg8owg0ow4wcocgw0w`
- `client_secret` = `1uiljdab2misc4owsc0kg0cw0kgw0k0gkgk0k8k488w8sskk4s`

(from `com.vitale.coredata.BuildConfig` and `Authentication.java:59-60`)

The `access_token` returned is saved as the "temp" token
(`authentication.saveTempToken`, key `temp_access_token`).

### Stage 2 — user login with email + password

`LoginRepositoryImpl.login()` -> `ApiHelper.login()` ->
`ApiService.login()`:

```java
@FormUrlEncoded
@POST("api/v2.0/{locale}/exerp/newAuth")
Object login(@Path("locale") String locale,
             @Field("access_token") String accessToken,   // temp token from stage 1
             @Field("password") String password,
             @Field("email") String email,
             @Field("appName") String appName);
```

Form fields:
- `access_token` = temp token from stage 1
- `email` = member email / username
- `password` = member password
- `appName` = `vivagym` (`NetworkModule.provideAppName()`)
- `{locale}` = e.g. `es`, `en`, `pt`

Response (`LoginResponse`) contains:
- `access_token` (long-lived access token)
- `refresh_token`
- `expires_in` (seconds)
- `token_type`, `app_name`, `app_color`, `code`, `message`

On success the app stores these and sets `isExerp = true`.

### Optional pre-check: does the email exist?

`POST email/check` (form: `email`, `appName`). Used by the UI before showing
the password screen.

### Token refresh

`GET api/email/refresh?refresh_token=<refresh_token>` returns a new
`LoginResponse` with fresh `access_token` / `refresh_token` / `expires_in`.
The okhttp `AuthInterceptor` triggers this automatically when the stored token
has expired, then retries the original request.

### Header injection

The okhttp `AuthInterceptor` adds `Authorization: Bearer <access_token>` to any
request whose URL path contains `api`, excluding paths containing
`forgot`, `refresh`, `app/info`, `newAuth`, `email`, `password`.

Tokens are stored in Android EncryptedSharedPreferences file `authentication_sec`
(keys: `access_token`, `refresh_token`, `expires_in`, `log_time`,
`temp_access_token`, `username`, `isExerp`).

## Full curl example

```sh
BASE=https://vivagym.myvitale.com
LOCALE=es            # es | en | pt

# 1) anonymous client-credentials token
TEMP=$(curl -s -X POST "$BASE/oauth/v2/token" \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"client_credentials",
       "client_id":"4_43uq8rgou3y88ckkk0sgg8c408w4gwsssg8owg0ow4wcocgw0w",
       "client_secret":"1uiljdab2misc4owsc0kg0cw0kgw0k0gkgk0k8k488w8sskk4s"}' \
  | jq -r .access_token)

# 2) log in with the member's email/password -> get bearer token
LOGIN=$(curl -s -X POST "$BASE/api/v2.0/$LOCALE/exerp/newAuth" \
  --data-urlencode "access_token=$TEMP" \
  --data-urlencode "email=YOUR_EMAIL" \
  --data-urlencode "password=YOUR_PASSWORD" \
  --data-urlencode "appName=vivagym")
ACCESS=$(echo "$LOGIN" | jq -r .access_token)
REFRESH=$(echo "$LOGIN" | jq -r .refresh_token)

# 3) fetch the gym-entry QR payload
curl -s "$BASE/api/v2.0/exerp/qr" -H "Authorization: Bearer $ACCESS"
# -> plain string; encode it as a QR code (e.g. with ZXing/qrencode)
```

## Source references (decompiled paths)

- `com/myvitale/api/Api.java` — QR endpoint (`api/v2.0/exerp/qr`)
- `com/myvitale/api/ApiService.java` — base URL + legacy bearer interceptor
- `com/myvitale/qr/presentation/...` — QR activity / presenter / interactor
- `com/vitale/coredata/data/api/ApiService.java` — OAuth + `newAuth` login
- `com/vitale/coredata/data/api/ApiHelperImpl.java` — `authenticate` / `login`
- `com/vitale/coredata/data/api/interceptor/AuthInterceptor.java` — bearer header + refresh
- `com/vitale/coredata/data/storage/Authentication.java` — client id/secret + token store
- `com/vitale/coredata/data/models/LoginResponse.java`, `OauthModel.java`
- `com/vitale/coredata/BuildConfig.java` — client credentials, grant type
