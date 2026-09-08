// Package community_subscriber implements the push-notification subscriber
// for community_event_bus. It owns recipient resolution, per-user preference
// gating, push copy assembly, and the soft-delete gate that suppresses
// notifications for deleted communities. See README.md for the contract.
package community_subscriber // snake_case package name matches the docs/issues/510 plan.
