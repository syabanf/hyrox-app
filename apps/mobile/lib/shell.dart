import 'dart:async';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'permissions.dart';
import 'package:webview_flutter/webview_flutter.dart';
import 'package:webview_flutter_android/webview_flutter_android.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:webview_flutter_wkwebview/webview_flutter_wkwebview.dart';

import 'main.dart' show brandBeige, brandInk, brandLime;

/// Where the member app lives.
///
/// Overridable at build time so a debug build can point at a laptop:
///
///     flutter run --dart-define=NUHABIT_URL=http://192.168.1.10:8088
///
/// Note the http there is only usable because the debug manifests permit
/// cleartext to a local address; a release build talks https or nothing.
const appUrl = String.fromEnvironment(
  'NUHABIT_URL',
  defaultValue: 'https://nuhabit.reddie.id',
);

/// Hosts the shell will render itself.
///
/// Anything else — a payment provider, a map, a link somebody pasted into a
/// class description — opens in the system browser. A webview that swallows
/// every link is how members end up stranded on a page with no address bar
/// and no way back.
final _ownHosts = <String>{Uri.parse(appUrl).host};

class WebShell extends StatefulWidget {
  const WebShell({super.key});

  @override
  State<WebShell> createState() => _WebShellState();
}

class _WebShellState extends State<WebShell> {
  late final WebViewController _controller;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _controller = _buildController();
    _load();
  }

  WebViewController _buildController() {
    // The platform-specific parameters exist because each engine needs
    // something the other does not: Android needs a filesystem for DOM
    // storage, WKWebView needs to be told that inline media is allowed.
    late final PlatformWebViewControllerCreationParams params;
    if (WebViewPlatform.instance is WebKitWebViewPlatform) {
      params = WebKitWebViewControllerCreationParams(
        allowsInlineMediaPlayback: true,
        mediaTypesRequiringUserAction: const <PlaybackMediaTypes>{},
      );
    } else {
      params = const PlatformWebViewControllerCreationParams();
    }

    final controller = WebViewController.fromPlatformCreationParams(params)
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setBackgroundColor(brandBeige)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) => setState(() {
            _loading = true;
            _error = null;
          }),
          onPageFinished: (_) => setState(() => _loading = false),
          onWebResourceError: (error) {
            // Only a failure of the page itself is worth a full error screen.
            // A stylesheet that 404s should not replace a working app with an
            // apology.
            if (!error.isForMainFrame!) return;
            setState(() {
              _loading = false;
              _error = _describe(error);
            });
          },
          onNavigationRequest: (request) {
            final target = Uri.tryParse(request.url);
            if (target == null) return NavigationDecision.prevent;
            if (_ownHosts.contains(target.host)) {
              return NavigationDecision.navigate;
            }
            // tel:, mailto:, maps:, another site — the phone knows what to do
            // with these and the shell does not.
            unawaited(_openExternally(target));
            return NavigationDecision.prevent;
          },
        ),
      );

    final platform = controller.platform;
    if (platform is AndroidWebViewController) {
      // The whole reason this shell exists rather than a bookmark.
      //
      // The web app asks for location through the standard browser API. On
      // Android a webview refuses that request by default and the page is
      // told "denied" — indistinguishable, from the page's side, from a user
      // saying no. So the shell asks the OS, then answers the page.
      platform.setGeolocationPermissionsPromptCallbacks(
        onShowPrompt: (request) async {
          final granted = await NativePermissions.requestLocation();
          // retain mirrors allow: remembering a refusal would mean the member
          // never gets asked again after changing their mind in Settings.
          return GeolocationPermissionsResponse(allow: granted, retain: granted);
        },
      );
      AndroidWebViewController.enableDebugging(kDebugMode);
      platform.setMediaPlaybackRequiresUserGesture(false);
    }
    return controller;
  }

  Future<void> _openExternally(Uri target) async {
    if (await canLaunchUrl(target)) {
      await launchUrl(target, mode: LaunchMode.externalApplication);
    }
  }

  Future<void> _load() async {
    final connection = await Connectivity().checkConnectivity();
    if (connection.every((c) => c == ConnectivityResult.none)) {
      setState(() {
        _loading = false;
        _error = 'No connection. NüHabit needs the internet to show your '
            'classes and your balance.';
      });
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    await _controller.loadRequest(Uri.parse(appUrl));
  }

  String _describe(WebResourceError error) {
    return switch (error.errorType) {
      WebResourceErrorType.hostLookup =>
        'Could not reach NüHabit. Check your connection and try again.',
      WebResourceErrorType.timeout =>
        'NüHabit took too long to answer. Try again in a moment.',
      WebResourceErrorType.connect =>
        'Could not connect to NüHabit. It may be down for a moment.',
      _ => 'Something went wrong loading NüHabit.',
    };
  }

  @override
  Widget build(BuildContext context) {
    // The hardware back button walks the web app's history first and only
    // leaves the app when there is nothing behind it. Without this, back
    // closes the app from any screen, which on Android reads as a crash.
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) async {
        if (didPop) return;
        if (await _controller.canGoBack()) {
          await _controller.goBack();
          return;
        }
        // Nothing behind us in the web app, so back means what it means
        // everywhere else on Android: put the app in the background. Popping
        // the route would do nothing at all — this is the only route.
        await SystemNavigator.pop();
      },
      child: Scaffold(
        backgroundColor: brandBeige,
        body: SafeArea(
          bottom: false,
          child: _error != null
              ? _ErrorView(message: _error!, onRetry: _load)
              : Stack(
                  children: [
                    WebViewWidget(controller: _controller),
                    if (_loading) const _Splash(),
                  ],
                ),
        ),
      ),
    );
  }
}

/// Shown while the first paint is on its way.
///
/// The brand ground rather than a spinner on white: the web app's own
/// background is this colour, so the handover is invisible instead of a flash
/// of white between the splash and the page.
class _Splash extends StatelessWidget {
  const _Splash();

  @override
  Widget build(BuildContext context) {
    return Container(
      color: brandBeige,
      child: const Center(
        child: SizedBox(
          width: 28,
          height: 28,
          child: CircularProgressIndicator(strokeWidth: 2.5, color: brandInk),
        ),
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.wifi_off_rounded, size: 40, color: brandInk),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 15,
                height: 1.4,
                color: brandInk,
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: onRetry,
              style: FilledButton.styleFrom(
                backgroundColor: brandLime,
                foregroundColor: brandInk,
                padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 14),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                ),
              ),
              child: const Text(
                'Try again',
                style: TextStyle(fontWeight: FontWeight.w800),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
