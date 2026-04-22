# KeyBank
An one-time Cryptography Key generation and retrieval service

# Note Sharing
Traditional secure note sharing service store the message on their servers and promise to destroy it after one fetch.

KeyBank applies an opposite model.

KeyBank generate the cryptography key pair for you, sends you the public key to encrypt the message. The ciphertext is held in the share url directly.

When fetching, KeyBank would fetch the private key and decrypt the message on the web interface.

The message is never sent to the server, therefore reducing the legal problem on storing and hosting user data, at the price of limited message length.

# Feature
- Zero user data on server
	- No legal problem for server
	- No Data Leak even when server hacked
- Fully encrypted procedure
	- Public key and private key are sent in two different access
	- No Eavesdropping 
- Every time one new keypair
	- No ciphertext only attack on consecutive use

# Credit
This project is inspired by many secure note sharing app on the Internet, especially the [ tutorial ](https://dusted.codes/building-a-secure-note-sharing-service-in-go) made by [Dusted Codes Limited](https://dusted.codes/about) .
	
