package service

// TODO: implement PredictionService.
//
// Critical business rule: a user may not submit or modify a prediction
// after the match's kick-off time. This deadline check must happen inside
// the service, not in the handler, so that it is enforced regardless of
// which transport (HTTP, gRPC, CLI) invokes the service.
