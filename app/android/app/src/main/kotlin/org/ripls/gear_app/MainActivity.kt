package org.ripls.app

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        ConversationShortcutsPlugin(applicationContext).register(flutterEngine)
        AudioSessionPlugin(applicationContext).register(flutterEngine)
    }
}
