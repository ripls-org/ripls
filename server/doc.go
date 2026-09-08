// Command server is the main Ripls application server. It wires together all
// RPC services, starts the Connect/HTTP server, runs startup jobs, and handles
// graceful shutdown on SIGINT or SIGTERM.
package main
