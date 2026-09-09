import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'shell.dart';

/// The NüHabit member app, wrapped for the phone.
///
/// The web app is the product; this is the shell that lets it be installed
/// from a store, hold a location permission, and behave like an app when the
/// hardware back button is pressed. It deliberately renders nothing of its
/// own beyond a splash and an error screen — every pixel a member sees is the
/// web app, so there is one place to change a screen rather than three.
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setSystemUIOverlayStyle(
    const SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: Brightness.dark,
      statusBarBrightness: Brightness.light,
    ),
  );
  runApp(const NuHabitApp());
}

/// Brand ground, from packages/ui/src/tokens.css. Kept in sync by hand
/// because two colours is not worth a build step, but they are the same two.
const brandInk = Color(0xFF00281A);
const brandBeige = Color(0xFFF3ECE2);
const brandLime = Color(0xFFDAFF59);

class NuHabitApp extends StatelessWidget {
  const NuHabitApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'NüHabit',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: brandInk,
          surface: brandBeige,
        ),
        scaffoldBackgroundColor: brandBeige,
        useMaterial3: true,
      ),
      home: const WebShell(),
    );
  }
}
