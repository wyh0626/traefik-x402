# Real Base Sepolia payment test

This profile loads the plugin into a real Traefik process, protects a local
upstream, and settles one payment through the public x402 test facilitator.

Payment settings:

- Network: Base Sepolia (`eip155:84532`)
- Asset: Circle test USDC (`0x036CbD53842c5426634e7929541eC2318f3dCF7e`)
- Price: `0.001` test USDC (`1000` atomic units)
- Receiver: supplied by you through `PAY_TO`
- Endpoint: `http://127.0.0.1:18080/paid`

Testnet assets have no cash value, but a successful test is still an
irreversible testnet transaction.

## Prerequisites

- Bash, curl, and Go 1.24 or newer.
- Traefik v3.7 on `PATH`, or an explicit `TRAEFIK_BIN`.
- A dedicated payer test wallet containing Base Sepolia test USDC.
- A receiver EVM address.

Circle test USDC is available from the
[Circle Faucet](https://faucet.circle.com/). Select USDC and Base Sepolia, then
send it to the dedicated payer wallet. If MetaMask does not display the balance,
import the token contract shown above while connected to Base Sepolia.

Never use a mainnet wallet or a wallet holding assets with monetary value.

## 1. Start Traefik and the local upstream

From the repository:

```bash
cd e2e/real
PAY_TO=0xYourReceiverAddress ./run.sh
```

If Traefik is not on `PATH`:

```bash
PAY_TO=0xYourReceiverAddress \
TRAEFIK_BIN=/absolute/path/to/traefik \
./run.sh
```

`run.sh` validates `PAY_TO`, generates dynamic configuration in a temporary
directory, starts the local upstream, and loads the local plugin. It does not
write a wallet private key anywhere.

## 2. Confirm the unpaid challenge

In a second terminal:

```bash
curl -i http://127.0.0.1:18080/paid
```

Expected result: HTTP `402` with `PAYMENT-REQUIRED`. No signature or blockchain
transaction is created by this request.

## 3. Make exactly one paid request

```bash
cd e2e/real
./pay.sh
```

Before requesting a key, the script reads the live `PAYMENT-REQUIRED` challenge
and displays its network, asset, atomic amount, and receiver. It continues only
after you type `PAY`. The dedicated test-wallet key is read without echo, passed
only to the Go x402 client process, and never written to a file.

The self-contained client under `e2e/real/client` uses the tagged x402 Go v2
SDK. Its dependencies may be downloaded on the first run.

Expected result:

- HTTP response contains `"ok": true` and the verified payer.
- `PAYMENT-RESPONSE` contains `success: true` and a transaction hash.
- The client prints a BaseScan transaction link.
- The payer loses `0.001` Base Sepolia test USDC.
- The configured receiver gains `0.001` Base Sepolia test USDC.

Every successful `./pay.sh` run creates another payment.

## Troubleshooting

- Confirm the endpoint returns 402 before running `pay.sh`.
- Confirm both wallet and token are on Base Sepolia, not Base mainnet.
- Import Circle test USDC into the wallet if the balance is hidden.
- Check the public facilitator's `/supported` response if the scheme or network
  is rejected; public testnet support can change.
- Use the printed [Base Sepolia explorer](https://sepolia.basescan.org/) link to
  inspect a successful transaction.
