import 'dart:io';

import 'package:flutter/services.dart';

/// The OS permissions the shell has to hold on the web app's behalf.
///
/// Only location, and only on Android. iOS grants a WKWebView the app's own
/// location permission automatically once Info.plist declares a usage string,
/// so there is nothing for this to do there — and a channel call to an
/// unimplemented handler would just throw.
class NativePermissions {
  static const _channel = MethodChannel('id.nuhabit/permissions');

  /// True if the app already holds it. Never prompts.
  static Future<bool> hasLocation() async {
    if (!Platform.isAndroid) return true;
    return await _channel.invokeMethod<bool>('hasLocation') ?? false;
  }

  /// Asks, showing the system prompt if it has not been answered before.
  ///
  /// A refusal is a `false`, not an exception: being told no is an ordinary
  /// outcome here, and the web app already has a screen for it.
  static Future<bool> requestLocation() async {
    if (!Platform.isAndroid) return true;
    try {
      return await _channel.invokeMethod<bool>('requestLocation') ?? false;
    } on PlatformException {
      return false;
    }
  }
}
