# NüHabit mobile

A Flutter shell around the member web app, so it can be installed from a
store, hold OS permissions, and behave like an app when the hardware back
button is pressed.

It renders nothing of its own beyond a splash and an error screen. Every
pixel a member sees is `apps/member`, which means a screen is changed in one
place rather than three — and the shell only needs shipping again when the
native surface changes, not when the product does.

## What it is for

A bookmark to the PWA would give you most of this. It would not give you:

- **Location.** The web app asks through the browser API. An Android webview
  refuses that outright unless the *app* holds `ACCESS_FINE_LOCATION`, and the
  page is simply told "denied" — indistinguishable, from the page's side, from
  a member saying no. The shell asks Android, then answers the webview.
- **The back button.** Android's back walks the web app's history first and
  only backgrounds the app when there is nothing behind it.
- **Links that leave.** Anything not on the app's own host opens in the system
  browser. A webview that swallows every link strands people on a page with no
  address bar.
- A store listing, an icon, and a splash that is the web app's own background
  rather than a flash of white.

## Running it

Against the deployed site:

```bash
flutter run
```

Against a laptop, which is what you want while working on `apps/member`:

```bash
flutter run --dart-define=NUHABIT_URL=http://10.0.2.2:8088   # Android emulator
flutter run --dart-define=NUHABIT_URL=http://localhost:8088  # iOS simulator
```

`10.0.2.2` is how an Android emulator reaches the host. Cleartext to it is
permitted by `android/app/src/debug/`, which is not merged into a release
build — a shipped app talks https or nothing.

**Location will not work over http.** Browsers refuse the geolocation API off
a secure origin, and the web app says so rather than looking broken. To
exercise location you need an https origin: the deployed site, or a tunnel.

## Building

```bash
flutter build apk --release
flutter build appbundle --release   # for Play
flutter build ipa --release         # needs a configured Xcode
```

## Getting an APK

`.github/workflows/mobile-android.yml` builds one. **Actions → Mobile ·
Android → Run workflow** asks which deployment the app should open and
whether you also want the Play bundle, then leaves the APK as an artifact on
the run.

It also runs on any pull request that touches `apps/mobile`, and on a
`mobile-v*` tag — the tag additionally publishes a GitHub release with the APK
and the `.aab` attached:

```bash
git tag mobile-v1.0.0 && git push origin mobile-v1.0.0
```

### Signing

Without a key, a release build is signed with the **debug** key. It installs
fine and is what you want for putting a build on a phone this afternoon; Play
will refuse it, and it can never update an APK that was signed properly. The
workflow summary says which kind came out, every time.

To sign properly, create a key once and put it in repository secrets:

```bash
keytool -genkey -v -keystore upload-keystore.jks \
  -keyalg RSA -keysize 2048 -validity 10000 -alias upload
base64 -i upload-keystore.jks | pbcopy   # → ANDROID_KEYSTORE_BASE64
```

| Secret | |
|---|---|
| `ANDROID_KEYSTORE_BASE64` | the keystore, base64 |
| `ANDROID_KEYSTORE_PASSWORD` | store password |
| `ANDROID_KEY_ALIAS` | `upload`, above |
| `ANDROID_KEY_PASSWORD` | key password |

Keep the keystore file itself somewhere safe and backed up. Losing it means
never updating the app on Play again under that listing — a new key is a new
app. Locally the same thing is `android/key.properties`, which is gitignored
along with `*.jks`.

## The native surface

Two files, and they are the whole reason this exists.

- `android/.../MainActivity.kt` — a method channel that requests the location
  permission. Written by hand rather than taken from a permissions plugin:
  `permission_handler` compiles only against an SDK this toolchain cannot
  currently address (see below), and this is forty lines. A dependency that
  dictates your Android SDK version should be earning more than one permission
  request.
- `lib/permissions.dart` — the Dart side. iOS is a no-op: WKWebView inherits
  the app's own location permission once `Info.plist` declares a usage string,
  so there is nothing to ask for.

Permissions are declared in `AndroidManifest.xml` and `ios/Runner/Info.plist`.
The iOS usage strings are shown verbatim in the system prompt, so they say
what the app does with the permission rather than restating its name.

## Known limits

- **iOS is unverified.** It is wired — Info.plist strings, WKWebView params —
  but Xcode is not fully configured on the machine this was built on, so no
  iOS build has ever run. Expect to fix something on the first `flutter build
  ipa`.
- **`<input type="file">` does nothing on Android.** `webview_flutter` does not
  expose `onShowFileChooser`, so the "add a photo" control in the recorder is
  inert inside the shell. It works in a browser. Fixing it means either
  patching the platform view or moving to `flutter_inappwebview`.
- **Android SDK pinning.** `android/build.gradle.kts` pins every subproject to
  `compileSdk 36`. The SDK installed here reports its API level as `37.0` —
  the new minor-versioned scheme — which this AGP cannot address by the hash
  string `android-37`. Remove the pin once AGP understands minor versions.
- **The home screen's tile grid mis-renders** in the Android webview: tiles
  overlap and repeat. The same page is correct in desktop Chromium at the same
  width, and every other screen tested — including the OpenLayers map — is
  correct in the shell. It is a web-app CSS problem that only Android's
  webview shows, not a shell bug, and it is not yet diagnosed.

## What has been verified

On an Android 15 emulator, against both a local dev server and the deployed
site:

- Loads the member app over http and https, including a deep path
  (`/train/heatmap`).
- OpenLayers/OSM maps render correctly, tracks and all.
- The web app's insecure-origin branch fires over http, with its own message,
  rather than failing silently.

Not yet exercised end to end: the location grant itself, which needs an https
origin serving a build with the locate control; the back button; and every
part of iOS.
