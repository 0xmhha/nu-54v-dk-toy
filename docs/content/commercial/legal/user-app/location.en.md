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

This edition of the app has no function that reads the phone’s current location from GPS or the network. It does not send a location to a server.

## Bluetooth and the location permission

The app finds nearby boards and stores over Bluetooth. On Android 12 and later it declares to the system that scan results are not used to find your location.

On Android 11 and earlier a Bluetooth scan needs the system’s location permission. It is used only for the scan.

## Information that hints at a location

Connecting to a nearby store or ordering there leaves that store’s record on the store server, which shows the place and time of the visit.

Choosing a region or viewing the map in travel information shows the public information server that region and map area. A server you connect to can guess a rough area from the IP address.

Itineraries, reviews and visit checks contain places. They go to the travel server only if you agreed.

## DApp pages

Location requests of DApp sites opened inside the app are blocked.

## Keeping and contact

The privacy policy states how long these records are kept.

If the app comes to use your current location, this notice gets a new edition and you are asked separately first.

Contact: {{publisher.contact}}
