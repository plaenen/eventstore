I would like to extend the protoc plugin with the option to create an AggregateCommnand Handler for aggregates, I would
  define an option on the service allowing the creation of an interface to be implemented e.g.  

```proto
service AccountService {
  option (eventsourcing.aggregate) = {
    aggregate_root_message: "Account" // Reference to the aggregagte message
  };

  // OpenAccount creates a new bank account
  rpc OpenAccount(OpenAccountRequest) returns (OpenAccountResponse);
}
```

would generate:

```go 
interface AccountServiceCommandHandler struct {
    OpenAccount(req *OpenAccountRequest, opt ...CommandHandlerOpt) (OpenAccountResponse, error)
}
```

it would also implement an "UnImplementedAccountServiceCommandHandler" which I can embed in the actual implementation

The CommandHandlerOpt could be used to pass in:
- WithPrincipal
- WithHeader
- With....

Does this make sense, suggest how to solve for this, think the generator has options atm would like to use this also as an option.