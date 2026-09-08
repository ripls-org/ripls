import AVFoundation
import Flutter
import FirebaseAuth
import GoogleMaps
import UIKit

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  // Retained handler for the `ripls/audio_session` channel (#2231).
  private let audioSessionPlugin = AudioSessionPlugin()

  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  // Google Maps SDK init bridge (#2188). Dart calls
  // 'org.ripls.app/google_maps_init#provideAPIKey' with the
  // --dart-define value before runApp; we forward it to
  // GMSServices.provideAPIKey before any GMSMapView can mount. Same
  // secret-flow shape as MAPBOX_ACCESS_TOKEN (--dart-define → Dart →
  // native SDK). Setting up the channel in didInitializeImplicitFlutterEngine
  // (rather than didFinishLaunchingWithOptions) guarantees the
  // engineBridge.binaryMessenger is ready.
  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    let channel = FlutterMethodChannel(
      name: "org.ripls.app/google_maps_init",
      binaryMessenger: engineBridge.applicationRegistrar.messenger()
    )
    channel.setMethodCallHandler { (call: FlutterMethodCall, result: @escaping FlutterResult) in
      if call.method == "provideAPIKey", let key = call.arguments as? String, !key.isEmpty {
        GMSServices.provideAPIKey(key)
        result(nil)
      } else {
        result(FlutterMethodNotImplemented)
      }
    }

    // Audio session category bridge (#2231). Replaces the audio_session
    // plugin, whose binary referenced the microphone requestRecordPermission
    // API and tripped Apple's ITMS-90683 purpose-string check on an app that
    // never records audio. This handler only sets AVAudioSession categories.
    // Registered via applicationRegistrar.messenger() like the Google Maps
    // bridge above — FlutterApplicationRegistrar is not a FlutterPluginRegistrar,
    // so we wire the channel directly rather than via FlutterPlugin.
    let audioChannel = FlutterMethodChannel(
      name: "ripls/audio_session",
      binaryMessenger: engineBridge.applicationRegistrar.messenger()
    )
    audioChannel.setMethodCallHandler {
      [weak self] (call: FlutterMethodCall, result: @escaping FlutterResult) in
      self?.audioSessionPlugin.handle(call, result: result)
    }
  }

  // Forward incoming URLs to Firebase Auth so it can handle the reCAPTCHA
  // callback during phone number verification on iOS.
  override func application(
    _ app: UIApplication,
    open url: URL,
    options: [UIApplication.OpenURLOptionsKey: Any] = [:]
  ) -> Bool {
    if Auth.auth().canHandle(url) {
      return true
    }
    return super.application(app, open: url, options: options)
  }
}

/// Native handler for the `ripls/audio_session` channel. Sets the app's
/// `AVAudioSession` category so feed-video playback coexists with other apps'
/// audio (issue #1250): `.ambient` + mixWithOthers when muted, `.playback` +
/// duckOthers when the user unmutes a feed video.
///
/// This deliberately references **no** microphone API (no
/// `requestRecordPermission`, no `AVCaptureDevice`) — the point of issue #2231.
/// The app plays audio but never records, so it must not require an
/// `NSMicrophoneUsageDescription` purpose string. Keep it that way.
final class AudioSessionPlugin: NSObject {
  func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "configureMuted":
      configure(category: .ambient, options: .mixWithOthers, result: result)
    case "configureAudible":
      configure(category: .playback, options: .duckOthers, result: result)
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private func configure(
    category: AVAudioSession.Category,
    options: AVAudioSession.CategoryOptions,
    result: @escaping FlutterResult
  ) {
    let session = AVAudioSession.sharedInstance()
    do {
      try session.setCategory(category, mode: .default, options: options)
      try session.setActive(true)
      result(nil)
    } catch {
      result(
        FlutterError(
          code: "audio_session_error",
          message: error.localizedDescription,
          details: nil
        )
      )
    }
  }
}
