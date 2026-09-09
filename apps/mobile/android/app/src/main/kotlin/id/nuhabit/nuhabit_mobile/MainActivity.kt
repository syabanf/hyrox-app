package id.nuhabit.nuhabit_mobile

import android.Manifest
import android.content.pm.PackageManager
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

/**
 * The one native thing this shell needs: the OS location permission.
 *
 * The web app asks for location through the standard browser API. An Android
 * webview refuses that outright unless the *app* holds ACCESS_FINE_LOCATION,
 * and the page is simply told "denied" — indistinguishable, from the page's
 * side, from the member saying no. So Dart asks this, this asks Android, and
 * the answer goes back to the webview.
 *
 * Written here rather than taken from a permissions plugin because the plugin
 * that does this compiles only against an SDK the toolchain cannot currently
 * address, and this is forty lines. A dependency that dictates your Android
 * SDK version should be earning more than one permission request.
 */
class MainActivity : FlutterActivity() {
    private var pending: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "hasLocation" -> result.success(granted())
                    "requestLocation" -> requestLocation(result)
                    else -> result.notImplemented()
                }
            }
    }

    private fun granted(): Boolean =
        ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED ||
            ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_COARSE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED

    private fun requestLocation(result: MethodChannel.Result) {
        if (granted()) {
            result.success(true)
            return
        }
        // Only one request may be outstanding: a second prompt while the first
        // is on screen leaves a Result that is never completed, and Dart waits
        // for it forever.
        if (pending != null) {
            result.success(false)
            return
        }
        pending = result
        ActivityCompat.requestPermissions(
            this,
            arrayOf(
                Manifest.permission.ACCESS_FINE_LOCATION,
                Manifest.permission.ACCESS_COARSE_LOCATION,
            ),
            REQUEST_LOCATION,
        )
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_LOCATION) return
        // Coarse alone is a yes. A member who granted "approximate" has agreed
        // to be located; refusing them because it is not precise would be the
        // app overruling a choice the OS offered them.
        val allowed = grantResults.any { it == PackageManager.PERMISSION_GRANTED }
        pending?.success(allowed)
        pending = null
    }

    private companion object {
        const val CHANNEL = "id.nuhabit/permissions"
        const val REQUEST_LOCATION = 4001
    }
}
