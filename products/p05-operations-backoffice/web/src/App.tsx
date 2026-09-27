import { DEVICE_SIGNED_TYPES, PROTOCOL_VERSION } from "@nu54/protocol";

// Placeholder shell for the back-office screens designed in p05/design.md §9.
export function App() {
  return (
    <main>
      <h1>NU54 Back-office</h1>
      <p>
        Protocol v{PROTOCOL_VERSION}. Device signs: {DEVICE_SIGNED_TYPES.join(", ")}.
      </p>
      <p>Merchants, rentals and returns screens arrive next cycle on the opsd API.</p>
    </main>
  );
}
