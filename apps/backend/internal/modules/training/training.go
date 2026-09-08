// Package training is the athlete side of the member app: activities with a
// GPS track, the social graph around them, segments, clubs, gear, generated
// HYROX workouts and the race calendar.
//
// It owns the `training` schema. Member names, the exercise library and the
// studio's challenges all belong to other modules and arrive through the ports
// declared in service.go — this package never reads another schema.
package training
