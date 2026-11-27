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

{{publisher.name}} is responsible for personal data in the app. Contact: {{publisher.contact}}. Each store is responsible for its order records.

## What is stored on the store device

Device registration: store server address and certificates, store and device numbers, the public menu.

Orders in progress: items, quantities, amounts, random numbers, pickup number, comparison code, payment requests. They are kept so that an unfinished order can continue.

A device using the older payment way: board address and signature, transaction number, order time. Only the last 100 finished orders are kept. The store key and gas key are wrapped with a key of the secure storage.

The representative, business number, address and phone number printed on receipts, if the store entered them.

The app does not ask for a customer’s name, phone number, email or card number, and contains no advertising or usage analytics. On Android the app’s data is not part of the system backup.

## What is sent

To the store server: items and quantities, amounts, customer number, board signature and account address, pickup and delivery steps, receipt printing steps.

To a chain reading server (older payment way): board address, store address, order number, the signed transaction. To a receipt server: store address and order number.

Changing the reading server in Settings makes the device connect to it to check its network.

These servers can see the IP address of the store device.

Payments, deliveries and refunds stay on a public chain as addresses, amounts, times and a random order number, and cannot be deleted.

## Nearby customers

The device talks to a nearby customer’s phone or board over Bluetooth. It takes a phone’s requests over an authenticated link and passes only what the order and payment need to the store server.

## How long it is kept and deletion

Records on the device are removed with the app. On iOS some can stay in secure storage after the app is removed.

Store server order and payment records are kept for: {{retention.storeOrders}}. Server access logs: {{retention.serverLogs}}.

## Sharing, processors and transfers abroad

Recipients, processors and transfers abroad, with their basis: {{privacy.recipients}}.

## Rights and contact

Where and how to ask for access, correction, deletion or a stop of processing: {{privacy.requests}}. Rules for children: {{privacy.children}}.
