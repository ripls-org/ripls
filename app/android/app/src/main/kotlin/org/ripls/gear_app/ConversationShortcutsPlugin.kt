package org.ripls.app

import android.content.Context
import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.core.app.Person
import androidx.core.content.LocusIdCompat
import androidx.core.content.pm.ShortcutInfoCompat
import androidx.core.content.pm.ShortcutManagerCompat
import androidx.core.graphics.drawable.IconCompat
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

/**
 * Native bridge that publishes long-lived per-conversation shortcuts used by
 * Android's Conversation Space (Android 11+). Without these shortcuts, chat
 * notifications render with MessagingStyle but don't get the priority surface
 * — that requires a [ShortcutInfoCompat] with [Person] + [LocusIdCompat] +
 * `setLongLived(true)` + `setIsConversation()`.
 *
 * Channel: `ripls/conversation_shortcuts`
 *
 * Methods:
 *  - `pushDynamicShortcut({conversation_id, short_label, long_label,
 *    person_name, person_key, person_icon_bytes?, person_uri?})`
 *    Idempotent: subsequent calls with the same `conversation_id` update the
 *    existing shortcut. ShortcutManagerCompat handles LRU eviction when the
 *    system shortcut cap is reached.
 *  - `removeShortcut({conversation_id})`
 *
 * On versions below Android 7.1 the compat APIs are no-ops; callers don't
 * need to guard.
 */
class ConversationShortcutsPlugin(private val context: Context) :
    MethodChannel.MethodCallHandler {

    fun register(flutterEngine: FlutterEngine) {
        MethodChannel(
            flutterEngine.dartExecutor.binaryMessenger,
            CHANNEL_NAME,
        ).setMethodCallHandler(this)
    }

    @Suppress("UNCHECKED_CAST")
    override fun onMethodCall(
        call: io.flutter.plugin.common.MethodCall,
        result: MethodChannel.Result,
    ) {
        try {
            when (call.method) {
                "pushDynamicShortcut" -> {
                    pushDynamicShortcut(call.arguments as Map<String, Any?>)
                    result.success(null)
                }
                "removeShortcut" -> {
                    val args = call.arguments as Map<String, Any?>
                    val conversationId = args["conversation_id"] as? String
                    if (conversationId.isNullOrEmpty()) {
                        result.error(
                            ARG_ERROR,
                            "conversation_id is required",
                            null,
                        )
                        return
                    }
                    ShortcutManagerCompat.removeLongLivedShortcuts(
                        context,
                        listOf(conversationId),
                    )
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        } catch (e: Throwable) {
            result.error(INTERNAL_ERROR, e.message, null)
        }
    }

    private fun pushDynamicShortcut(args: Map<String, Any?>) {
        val conversationId = (args["conversation_id"] as? String).orEmpty()
        if (conversationId.isEmpty()) {
            throw IllegalArgumentException("conversation_id is required")
        }

        val shortLabel = (args["short_label"] as? String).orEmpty()
        val longLabel = (args["long_label"] as? String) ?: shortLabel
        val personName = (args["person_name"] as? String).orEmpty()
        val personKey = (args["person_key"] as? String) ?: conversationId
        val personUri = args["person_uri"] as? String
        val personIconBytes = args["person_icon_bytes"] as? ByteArray

        val personBuilder = Person.Builder()
            .setName(personName.ifEmpty { shortLabel })
            .setKey(personKey)
        if (!personUri.isNullOrEmpty()) {
            personBuilder.setUri(personUri)
        }
        if (personIconBytes != null) {
            val bitmap = BitmapFactory.decodeByteArray(
                personIconBytes,
                0,
                personIconBytes.size,
            )
            if (bitmap != null) {
                personBuilder.setIcon(IconCompat.createWithBitmap(bitmap))
            }
        }
        val person = personBuilder.build()

        // Tapping the shortcut opens the app to the conversation. The Dart
        // deep-link router (`routeNotificationPayload`) resolves the path.
        val intent = Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse("ripls://conversation/$conversationId")
            setPackage(context.packageName)
        }

        val builder = ShortcutInfoCompat.Builder(context, conversationId)
            .setShortLabel(shortLabel.ifEmpty { personName.ifEmpty { conversationId } })
            .setLongLabel(longLabel.ifEmpty { personName.ifEmpty { conversationId } })
            .setIntent(intent)
            .setPerson(person)
            .setLongLived(true)
            .setLocusId(LocusIdCompat(conversationId))
            .setCategories(setOf(SHARE_TARGET_CATEGORY))

        if (personIconBytes != null) {
            val bitmap = BitmapFactory.decodeByteArray(
                personIconBytes,
                0,
                personIconBytes.size,
            )
            if (bitmap != null) {
                builder.setIcon(IconCompat.createWithBitmap(bitmap))
            }
        }

        // ShortcutManagerCompat.pushDynamicShortcut is idempotent on
        // conversation_id and performs LRU eviction internally when the
        // system shortcut cap is exceeded.
        ShortcutManagerCompat.pushDynamicShortcut(context, builder.build())
    }

    companion object {
        const val CHANNEL_NAME = "ripls/conversation_shortcuts"

        // Marker category that enables share-sheet integration when other
        // apps share text — costs nothing extra for chat shortcuts.
        private const val SHARE_TARGET_CATEGORY =
            "android.shortcut.conversation"

        private const val ARG_ERROR = "argument_error"
        private const val INTERNAL_ERROR = "internal_error"
    }
}
