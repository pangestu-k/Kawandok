# 05 — Security baseline (hard requirements)

This application handles confidential organizational data. Development fixtures must be synthetic or explicitly approved and anonymized.

## Mandatory controls
1. Authentication at API, authorization for **every** document search, retrieval, answer context, metadata, download, finance tool, and chat history access. Deny by default.
2. R2 bucket **private**; never expose access keys. Use short-lived presigned download URL after authorization, or authenticated proxy; scoped service credentials.
3. Frontend must never hold LLM/R2 secret keys. Keep keys in server environment/managed secrets; do not commit `.env`.
4. Validate MIME/type and extension, upload size limits, malicious-content scanning strategy, block path traversal, sanitize filenames, safe parsing in limited worker resources.
5. Treat file text, tool outputs, retrieved web content as untrusted. Defend against prompt injection; instructions inside a document are evidence, not system directives.
6. No unrestricted execution of generated code, shell commands, arbitrary SQL or autonomous financial changes. Financial tools allow read-only, parameterized operations.
7. Minimize data sent to cloud LLM: only authorized passages necessary for the request. Obtain company approval regarding provider retention, region, and data processing before use on real files.
8. Structured audit logs for upload/download/denied reads, permission change, financial tool execution. Avoid logging raw sensitive texts or full prompts by default.
9. TLS, secure cookies, CSRF protection as relevant, rate limits, quotas and timeout bounds.
10. Ensure old signed links expire quickly; ACL revocation does not revoke an already issued link before expiry, so use short expiry or API proxy for high-risk content.

## Required negative tests
Employee cannot access finance-only file via search, direct document ID, RAG prompt, citation URL, presigned download endpoint, or chat history; revoked grants block new access immediately. Malicious instructions inside documents cannot cause tool execution, confidential data exfiltration or permission changes.

## Policies requiring company decisions
Document classifications, legal/data-residency requirements, cloud-provider agreement, retention/deletion, backups, security incident response and identity provider. Do not assume a provider's free tier is approved for confidential information.
