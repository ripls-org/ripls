// Package weather supplies per-day weather for the Home calendar: a coarse sky
// condition and a high temperature for each day at a given place.
//
// The design mirrors the stock-imagery subsystem (server/media): a
// provider-independent Provider interface, a cache wrapper that collapses calls
// to a per-place grid (weather for a lat/lng is identical for every viewer, so
// it is never fetched per user or per request), a fallback to climate normals
// beyond the forecast horizon, and a fake provider for tests.
//
// Open-Meteo is the MVP provider; it needs no API key and serves both the daily
// forecast and the climate-normals fallback. See docs/weather.md for the full
// picture (providers, the grid/cache, the horizon fallback, and config).
package weather
