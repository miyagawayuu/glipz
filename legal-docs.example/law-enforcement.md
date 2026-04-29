## Law Enforcement Request Policy

Edit this file to describe how your Glipz instance receives and reviews law enforcement requests.

### What We Can Provide

When a valid legal request is received, the operator may provide only data the server actually stores, such as account records, login or access metadata, report records, encrypted DM payloads, attachment metadata, and timestamps.

### Direct Messages

Direct messages and DM attachments are designed to be client-side encrypted. The server does not hold decrypted message bodies or decrypted attachment contents. A legal request may receive encrypted payloads and metadata, but not plaintext the server cannot access.

### Preservation

The operator may preserve relevant records when a valid preservation request is received. Preservation is separate from disclosure and should be time-limited.

### Operator Review Workflow

Operators should require a written request that identifies the requesting agency, jurisdiction, legal authority, target account or resource, requested date range, and requested data types. Do not disclose records based only on phone calls, informal chat messages, or unverifiable email.

Before exporting records, operators should review scope and necessity, create a preservation hold when appropriate, record whether user notice is permitted or prohibited, and export only the data types covered by the request. Disclosure packages include a manifest with section counts and SHA-256 hashes to help detect later modification.

### Access Metadata

Access metadata may include timestamps, IP addresses, user agents, and security-relevant account events retained by the server. Treat this metadata as personal data and disclose it only when it is within the verified request scope.

### Emergency Requests

Emergency requests involving imminent risk of death or serious physical harm should include the requesting agency, legal authority, affected users, requested records, and contact information for verification.

### User Notice

Where legally permitted and safe, the operator should consider notifying affected users before or after disclosure. Do not notify users when prohibited by law or when notification would create a safety risk.

### Contact

Add the contact method for law enforcement and emergency requests here.
