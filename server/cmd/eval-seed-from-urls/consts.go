package main

// extJPG is the extension written for every downloaded image. The alternate
// ".jpeg" spelling is accepted on input but never emitted.
//
// This lives in a sibling file rather than main.go because that file is at the
// 1,000-line gate (#1424); adding four lines there tripped it.
const extJPG = ".jpg"
