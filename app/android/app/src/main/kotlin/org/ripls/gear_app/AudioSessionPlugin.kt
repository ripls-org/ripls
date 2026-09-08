package org.ripls.app

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Build
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

/**
 * Native bridge that configures the app's audio focus so feed-video playback
 * coexists with other apps' audio (issue #1250). Replaces the `audio_session`
 * Flutter plugin, removed in #2231 because its iOS counterpart referenced the
 * microphone `requestRecordPermission` API and tripped Apple's ITMS-90683
 * purpose-string check on an app that never records. This handler touches only
 * [AudioManager] audio focus — no microphone, no recording.
 *
 * Channel: `ripls/audio_session`
 *
 * Methods (both idempotent; safe to call repeatedly):
 *  - `configureMuted`   — transient focus that may duck others, the closest
 *    analogue to "ambient mixing" (the app plays a muted feed video while
 *    other apps keep playing). Mirrors the prior `audio_session` config
 *    (`gainTransientMayDuck`, media/movie attributes).
 *  - `configureAudible` — exclusive `AUDIOFOCUS_GAIN` (the user explicitly
 *    unmuted a feed video). Mirrors the prior `audio_session` config.
 *
 * We request focus but do not react to focus-change callbacks, matching how the
 * app used `audio_session` (configure-only, no interruption handling).
 */
class AudioSessionPlugin(context: Context) : MethodChannel.MethodCallHandler {

    private val audioManager =
        context.getSystemService(Context.AUDIO_SERVICE) as AudioManager

    private val focusListener = AudioManager.OnAudioFocusChangeListener {}

    private var focusRequest: AudioFocusRequest? = null

    fun register(flutterEngine: FlutterEngine) {
        MethodChannel(
            flutterEngine.dartExecutor.binaryMessenger,
            CHANNEL_NAME,
        ).setMethodCallHandler(this)
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        try {
            when (call.method) {
                "configureMuted" -> {
                    requestFocus(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK)
                    result.success(null)
                }
                "configureAudible" -> {
                    requestFocus(AudioManager.AUDIOFOCUS_GAIN)
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        } catch (e: Throwable) {
            result.error(INTERNAL_ERROR, e.message, null)
        }
    }

    private fun requestFocus(focusGain: Int) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            // Replace any prior request before issuing a new one so the latest
            // mute/unmute state wins.
            focusRequest?.let { audioManager.abandonAudioFocusRequest(it) }
            val attributes = AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(AudioAttributes.CONTENT_TYPE_MOVIE)
                .build()
            val request = AudioFocusRequest.Builder(focusGain)
                .setAudioAttributes(attributes)
                .setOnAudioFocusChangeListener(focusListener)
                .build()
            focusRequest = request
            audioManager.requestAudioFocus(request)
        } else {
            @Suppress("DEPRECATION")
            audioManager.requestAudioFocus(
                focusListener,
                AudioManager.STREAM_MUSIC,
                focusGain,
            )
        }
    }

    companion object {
        const val CHANNEL_NAME = "ripls/audio_session"
        private const val INTERNAL_ERROR = "internal_error"
    }
}
