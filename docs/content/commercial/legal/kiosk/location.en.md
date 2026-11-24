---
id: location
language: en
title: About location
edition: draft-1
status: draft
effective: null
---

> This is an unreviewed draft and not legal advice. Blanks are filled in `publisher.json`. It is not distributed before they are filled and the text is reviewed.

## The app does not read your location

This edition of the app has no function that reads the device’s current location from GPS or the network. It does not send a location to a server.

## Bluetooth and the location permission

The app finds nearby customer phones and boards over Bluetooth. On Android 12 and later it declares to the system that scan results are not used to find a location.

On Android 11 and earlier a Bluetooth scan needs the system’s location permission. It is used only for the scan.

## Information that hints at a location

Order records name the store, so they show when a customer visited it. A server the device connects to can guess a rough area from the IP address.

## Contact

{{publisher.contact}}
