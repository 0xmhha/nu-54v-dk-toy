---
id: privacy
language: en
title: Privacy policy
edition: draft-1
status: draft
effective: null
---

> This is an unreviewed draft and not legal advice. Blanks are filled in `publisher.json`. It is not distributed before they are filled and the text is reviewed.

## Who is responsible

{{publisher.name}} is responsible for personal data in the app. Contact: {{publisher.contact}}.

A store or an operator runs the store server. It can differ for each store registered in the app.

## What the app does not ask for

Registering with a store and paying do not ask for your name, phone number or email. The app contains no advertising or usage analytics.

A store tells users apart by a random number and a certificate. This number and wallet addresses are linked to purchases, so they are treated as personal data.

## What is stored on the phone

Wallet record: board identifier, board account address and generation, language, the edition and time of documents you agreed to, chosen networks and servers, balances read, signatures and transactions in progress, DApp connections.

Store record: store server address and certificates, customer number, order numbers and states, amounts, store name, the seed of the order confirmation key, answers sent, scheduled notices.

Bluetooth address, name and wallet address of a rental board registered the older way.

Voice recorded in the travel feature with its list (at most 20), and confirmation values of deletion requests.

Wallet and order records are encrypted with a key of the phone’s secure storage. The rental board list, store registration and scheduled notices are in storage only this app reads. On Android the app’s data is not part of the system backup.

## What is exchanged with the board

The app talks to the board directly over Bluetooth. The PIN of a spending board is entered on the board only and never reaches the phone.

While a rental board is set up the older way, the board shows the PIN digits being entered in the app. The app does not store them.

## What is sent to the store server

For an order: customer number, store number, items and quantities, amount, the time and number of people for a booking, subscription agreements and changes.

For a payment: the quote identifier, the board signature, the board account address and generation, the public address for order confirmation.

After delivery: your answer (received, dispute, dispute withdrawn) with a signature. For a return: board account address, receiving address, amount, deadline, signature.

The store server can see the IP address you connect from. When you order over Bluetooth in the store, the same content passes through the store’s device.

## The chain and reading servers

To read balances and transactions the app sends your wallet address to the chosen reading server. That server can see the address together with your IP address. A personal transfer also sends the signed transaction.

Servers the app uses at first: api.test.stablenet.network, ethereum-sepolia-rpc.publicnode.com, base-sepolia.gateway.tenderly.co, mempool.space, api.devnet.solana.com, s.altnet.rippletest.net. Others run them under their own policies. You can change them in Settings, and a store or an operator can name other ones.

Payments, receipts, disputes and returns leave addresses, amounts, times and a random order number on a public chain. Anyone can see them and they cannot be deleted.

## What optional features send

DApp browser: a site you visit sees your IP address, and your wallet address if you approve the connection. The app blocks cookies and location requests.

Connecting another app (WalletConnect): the relay server receives your IP address and encrypted messages.

Travel information: the public information server sees the region you search, the map area and your IP address. Voice, transcription requests, itineraries, reviews and visit checks are sent to the travel server only if you agreed.

Cloud wallet: available only when configured for the deployment. The sign-in provider and the wallet server receive the values needed to sign in and sign.

Test market: the practice wallet address and transactions go to the test market server and the chain.

A share button sends what is on the screen to the app you choose.

## Permissions

Bluetooth: finding and connecting to boards and stores. Camera: reading QR codes. The app does not keep the picture. Microphone: travel voice recording. Notifications: order-ready and subscription notices. They are made on the phone, without an outside push server.

If you refuse a permission, features that do not need it still work.

## How long it is kept and deletion

Records on the phone are removed with the app. On iOS some can stay in secure storage after the app is removed.

Resetting a board erases its key, PIN and registration. Finished order records stay on the phone.

Store server order and payment records are kept for: {{retention.storeOrders}}. Server access logs: {{retention.serverLogs}}. Travel server records: {{retention.travelRecords}}.

Chain records cannot be deleted.

## Sharing, processors and transfers abroad

Recipients, processors and transfers abroad, with their basis: {{privacy.recipients}}.

## Your rights

The app shows stored orders, agreements and network settings. In the travel feature you can withdraw consent for each purpose and ask for deletion.

Where and how to ask for other access, correction, deletion or a stop of processing: {{privacy.requests}}.

Rules for children: {{privacy.children}}.
