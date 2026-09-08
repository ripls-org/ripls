#!/bin/bash

# Copy appropriate GoogleService-Info.plist based on build configuration
# This script is called from Xcode Build Phases (before Compile Sources)

set -e

# Copy to source directory so Flutter includes it in the bundle
PLIST_DESTINATION="${PROJECT_DIR}/Runner/GoogleService-Info.plist"

if [ "${CONFIGURATION}" == "Release" ]; then
    PLIST_SOURCE="${PROJECT_DIR}/config/release/GoogleService-Info.plist"
    echo "Using Release Firebase config"
else
    PLIST_SOURCE="${PROJECT_DIR}/config/dev/GoogleService-Info.plist"
    echo "Using Dev Firebase config"
fi

if [ ! -f "${PLIST_SOURCE}" ]; then
    echo "error: GoogleService-Info.plist not found at ${PLIST_SOURCE}"
    echo "error: Please download the appropriate config from Firebase Console"
    exit 1
fi

echo "Copying ${PLIST_SOURCE} to ${PLIST_DESTINATION}"
cp "${PLIST_SOURCE}" "${PLIST_DESTINATION}"
