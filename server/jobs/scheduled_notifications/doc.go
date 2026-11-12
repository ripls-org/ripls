// Package scheduled_notifications drives the reconciler/dispatcher pair
// that turns durable properties of in-flight entities (Experience start
// times, loan return dates) into push notifications fired at the right
// moment. See the #625 plan (archived) for the design
// and lifecycle.
package scheduled_notifications // underscore-style package name matches the proto kind family.
