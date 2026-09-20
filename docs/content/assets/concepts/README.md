# 제품 콘셉트 이미지 생성 기록

## three-month-maker-team-hero-nu54v-v2

파일: `three-month-maker-team-hero-nu54v-v2.png`

생성일: 2026-09-20. OpenAI 내장 image_gen 사용.

기존 팀 프로젝트 이미지를 편집 대상으로, `../nu-54v-dk-official.jpg`를 보드 외형의 참조 원본으로 사용했다. 작업대의 임의 녹색 PCB를 제거하고 NU-54V-DK의 길고 좁은 검정 PCB, 양쪽 핀 열, 중앙 모듈, 하단 디버거·USB 영역을 반영했다. 열린 하부 케이스에는 보드를 길이 방향으로 배치하고 고정 지지대, USB 개구부, 화면·버튼 배선 공간을 표현했다. 제조사가 완성 DK의 외곽 치수를 공개 문서 본문에 숫자로 명시하지 않아 공식 사진의 비율을 기준으로 생성했으며, 제작용 CAD 치수로 사용할 수 없다.

생성 원본: `three-month-maker-team-hero-nu54v-v2.png`

최종 프롬프트:

```text
Use case: precise-object-edit and compositing
Asset type: revised 16:9 Medium article hero image for a three-month NU-54V-DK maker project

Input images:
- Image 1 is the edit target. Preserve its warm cafe-workbench setting, wide editorial framing, tablet kiosk, smartphone companion app, three collaborators' hands, and realistic maker photography.
- Image 2 is the sole source of truth for the development board. It shows the real NUCODE NU-54V-DK front and back.

Primary request:
Rebuild Image 1 so every visible fictional green development board is removed and replaced by exactly one clearly recognizable NU-54V-DK based on Image 2. The replacement board must be a long, narrow BLACK PCB, approximately three times as long as it is wide, with two continuous rows of edge pin holes, the central shielded NU-54V module, dense components, indicator LEDs, switches, and the lower on-board DAP/debug/USB area in the same visual arrangement as the official reference. Do not create another green PCB and do not substitute a generic Nordic development kit.

Update the handheld enclosure to be designed around the NU-54V-DK's actual long, narrow form factor. The closed device in the hand should become slightly taller and slimmer, with realistic internal depth for the board, display, two physical buttons, wiring, fasteners, and connector clearance. On the workbench next to the hand, show a second view of the same enclosure as an OPEN 3D-printed lower shell containing the same NU-54V-DK board. The board must sit lengthwise inside the shell with modest clearance around its perimeter, four plausible standoffs or retention points, an accessible USB connector cutout at the correct board end, cable space for the display and buttons, and an unobstructed RF/antenna area. The open shell is there to prove that the case proportions and internal layout match the long narrow board.

Invariants:
Keep the wide 16:9 composition and cafe context. Keep the tablet kiosk and phone in roughly the same positions. Keep natural hands and warm workshop lighting. Keep a compact clip or lanyard feature on the handheld. Preserve realistic 3D-print layer texture and screw construction.

Accuracy constraints:
Use Image 2 faithfully for the NU-54V-DK color, silhouette, aspect ratio, pin rows, central module, major component zones, and branding placement. The board may be shown at a perspective angle but must remain recognizably the same product. Use visual proportions from Image 2; do not add numeric dimensions or imply an engineering drawing. The enclosure should have roughly 3–5 mm visual clearance around the PCB and enough depth above the tallest components. It must look physically manufacturable and serviceable.

Style/medium:
Photorealistic editorial product photography, premium but achievable maker prototype, practical rather than science fiction.

Composition/framing:
Wide landscape. Closed revised handheld centered in the foreground hand. Open enclosure with installed NU-54V-DK clearly visible on the cutting mat. Tablet kiosk in center-right, phone lower-right. Leave some calm negative space upper-left for Medium cropping.

Lighting/mood:
Warm natural workshop and cafe light, collaborative, credible, optimistic.

Constraints:
No cryptocurrency logos, no floating holograms, no dimension labels, no explanatory text, no watermark. One visible NU-54V-DK board only; do not show extra generic PCBs. Screens may use simple icons and abstract interface shapes without readable brand names.

Avoid:
green generic development board, square PCB, oversized desktop motherboard, Nordic nRF54L15 DK, transparent impossible enclosure, board intersecting walls, blocked USB port, implausibly tiny case, distorted hands, cyberpunk effects.
```

## three-month-maker-team-hero

파일: `three-month-maker-team-hero.png`

생성일: 2026-09-20. OpenAI 내장 image_gen 사용.

이 이미지는 세 명의 팀원이 NU-54V-DK 기반 휴대형 결제 기기, 카페 키오스크, 사용자 앱을 함께 만드는 장면을 설명하기 위한 AI 생성 콘셉트다. 실제 NU-54V-DK 보드, 완성 제품 또는 실제 작업 현장을 촬영한 사진이 아니다.

생성 원본: `three-month-maker-team-hero.png`

최종 프롬프트:

```text
Use case: product-mockup
Asset type: Medium article hero image for a three-month maker team project
Primary request: Create a photorealistic editorial product concept of a three-person maker team building a compact handheld crypto payment wallet around a small green Bluetooth development board. Show the finished concept and active prototyping together: a compact clip-style handheld device with a small OLED display and two physical buttons, a cafe kiosk tablet showing a simple drink order and test payment confirmation, and a smartphone companion app nearby. The device should feel useful for an international traveler paying at a small cafe.
Scene/backdrop: clean but lived-in electronics workbench in a small studio, with soldering tools, jumper wires, a development board, and a 3D-printed enclosure prototype
Subject: three collaborators' hands working together; no identifiable faces required; the handheld device is the visual focus
Style/medium: realistic product photography, premium but achievable maker prototype, not science fiction
Composition/framing: wide 16:9 landscape, device and tablet in the center-right, clear negative space on the upper-left for a Medium title crop
Lighting/mood: warm natural workshop lighting, practical, collaborative, optimistic
Color palette: charcoal enclosure, green PCB, warm wood desk, restrained blue interface accents
Constraints: no readable brand names, no cryptocurrency logos, no floating holograms, no impossible transparent electronics, no text, no watermark; the board shown is illustrative and must not impersonate an exact commercial board photo
Avoid: futuristic cyberpunk styling, luxury retail render, coins flying, excessive screens, distorted hands
```

2026-09-15 추가: [모션 장갑·인테리어 소품 최종 프롬프트](prompts-2026-09-15.md).

생성일: 2026-09-14. OpenAI 내장 image_gen 사용. 모두 실제 제작 전 콘셉트이며, 치수·기구·전기·비행 성능은 검증되지 않았다. 화면의 값은 예시다. 실제 NU-54V DK 보드 사진을 합성하거나 재현한 이미지가 아니다.

## core-01

파일: `core-01.png`

생성 원본: `core-01.png`

최종 프롬프트:

```text
Use case: product-mockup. Create a premium yet achievable DIY industrial design concept render, landscape 1536x1024. Real 3D-printed plastic textures, plausible fasteners, maker-quality engineering, beautiful editorial photography lighting. This is a product ideation illustration, not a manufactured or tested product. No invented PCB closeups, no logos of existing companies, no watermark. All text minimal and accurate. Include small "CONCEPT / NOT BUILT" caption at lower left.
Subject: CORE-01, a removable electronics cartridge shared by a game controller and a display robot figurine. ONE coherent tabletop scene clearly explaining the physical concept. At left a substantial 28 cm wide dual-analog-stick game controller in matte graphite with pale grey grips, four face buttons, D-pad, shoulder triggers, and an EMPTY vertically oriented rectangular cartridge docking bay in the middle. A removable black and teal cartridge, approximately 13 cm long, 4 cm wide, 2 cm thick, rests diagonally in front of the controller; it has a small OLED pixel face and is clearly the part that slots into the controller. At right stands a charming original 32 cm tall sci-fi maintenance robot model on a stable display pedestal, chunky articulated limbs, ivory and charcoal 3D-printed armor, tiny amber indicator lamps, no weapons. Its torso has a matching EMPTY rectangular docking bay of believable size for the same cartridge. Show exactly ONE physical cartridge in the whole image. Light etched labels adjacent to both slots say "CORE DOCK". Small title upper left "CORE-01", small subtitle "PLAY. DOCK. DISPLAY.".
Scene: warm designer/maker desk, soft neutral background, full view of controller, cartridge and robot, no cropped products, no floating exploded components. The concept must make it visually obvious the same electronics core can move between the two housings. Beautiful, sophisticated, believable proportions. Avoid Gundam or other licensed character likeness.
```

## memory-familiar

파일: `memory-familiar.png`

생성 원본: `memory-familiar.png`

최종 프롬프트:

```text
Use case: product-mockup. Create a premium yet achievable DIY industrial design concept render, landscape 1536x1024. Real 3D-printed plastic textures, plausible fasteners, maker-quality engineering, beautiful editorial photography lighting. This is a product ideation illustration, not a manufactured or tested product. No invented PCB closeups, no logos of existing companies, no watermark. All text minimal and accurate. Include small "CONCEPT / NOT BUILT" caption at lower left.
Subject: MEMORY FAMILIAR, an original palm-sized chunky portable voice-journal virtual-pet device. A 15 cm tall by 8 cm wide by 3 cm thick handheld gadget in muted sage and warm ivory, placed on a cafe table next to a small notebook and a fabric bag, enough space inside for a long development board. Large recessed rounded-square color screen shows a cute original pixel-art explorer creature with a small backpack, and three pixel collectible icons: bread, leaf, star. Display title "TODAY'S FINDS" and three slots. Physical controls: one prominent amber push-to-record button with a simple microphone symbol, a few tiny microphone grille holes nearby, two small navigation buttons below the display, wrist strap, visible 3D-print layer texture and assembly screws. A tiny REC status LED is off. This device records short voice memories, later syncing with a PC; do NOT show live cloud streams or real-time location.
Scene: warm afternoon cafe, candid close product shot, product fully visible and central, screen and record button easy to understand. Subtle small title at top "MEMORY FAMILIAR" and subtitle "YOUR DAY BECOMES AN ADVENTURE". No smartphone, no corporate characters, no holograms.
```

## dungeon-relic

파일: `dungeon-relic.png`

생성 원본: `dungeon-relic.png`

최종 프롬프트:

```text
Use case: product-mockup. A premium realistic DIY product concept render, landscape 1536x1024. Achievable 3D-printed hardware, real fasteners and subtle layer texture, thoughtful industrial design, beautiful editorial lighting. Small "CONCEPT / NOT BUILT" caption at bottom left. Minimal purposeful text only; do not add motivational slogans on props or in background. Not a tested manufactured product. No existing brand logos or fictional PCB closeups.
Subject: DUNGEON RELIC, a physical handheld puzzle/adventure gadget that turns room props into a treasure hunt. Foreground hero gadget is substantial, approximately 17 cm tall and 8 cm wide, rugged dark charcoal and antique brass colored plastic, like an archaeological science-fiction instrument. Rectangular small screen shows a simple pixel-art locked door and text "GATE 02" and "FIND THE SIGIL". Below it one tactile rotary dial and two round buttons, small warm amber indicator lights. At its top a CLEAR simple flat NFC sensing area marked with a small geometric rune; no compass or GPS. The gadget is held in one believable adult hand, touching a circular rune token on a small wooden puzzle chest on a table. Two other rune tokens are visibly placed nearby on a book and a small original knight figurine pedestal, suggesting three physical checkpoints. The chest lid is slightly open with a warm LED glow inside but no fantastical light beams. A few subtle clue cards. Warm evening living room in background, full device visible.
Typography: title "DUNGEON RELIC" at upper left, small subtitle "A ROOM BECOMES AN ADVENTURE". Physical tabletop adventure, grounded scale, not a weapon, no fake magical holograms. Treat light as real LEDs and screens, not CGI magic.
```

## tiny-cockpit

파일: `tiny-cockpit.png`

생성 원본: `tiny-cockpit.png`

최종 프롬프트:

```text
Use case: product-mockup. A premium realistic DIY product concept render, landscape 1536x1024. Achievable 3D-printed hardware, real fasteners and subtle layer texture, thoughtful industrial design, beautiful editorial lighting. Small "CONCEPT / NOT BUILT" caption at bottom left. Minimal purposeful text only; do not add motivational slogans on props or in background. Not a tested manufactured product. No existing brand logos or fictional PCB closeups.
Subject: TINY COCKPIT, a compact custom lap-sized gaming control console for a spaceflight or vehicle PC game. A 36 cm wide low rectangular graphite console with comfortable rounded edges and a padded underside sits on a real grey sofa cushion. On the right a single substantial joystick with a trigger and thumb button, on the left a short fore-aft throttle lever in a slotted track. Center strip has a small rectangular monochrome status screen, four robust metal toggle switches, two tactile pushbuttons, and ONE red flip-up guard over an "ARM" button. At rest power is indicated by subdued teal and amber LEDs. Original clean industrial design, realistic internal space, cable-free appearance conveys intended wireless operation but no battery runtime claims. 3D-printed shells with believable screws, no costly force feedback mechanisms. Soft-focus TV/monitor in background shows an abstract generic spacecraft cockpit in a game, no existing game logos. Do not show hands or people.
Full console visible, compelling three-quarter front view, cozy evening gaming atmosphere, restrained retro-futuristic aesthetic. Small title upper left "TINY COCKPIT" and subtitle "YOUR SOFA. YOUR FLIGHT DECK.". Show separate actual joystick and throttle unmistakably, not a generic gamepad.
```

## scout-01

파일: `scout-01.png`

생성 원본: `scout-01.png`

최종 프롬프트:

```text
Use case: product-mockup.
Create a realistic landscape 1536x1024 maker-project industrial design concept render called SCOUT-01.
Scene: calm daylight maker workbench. Show a small civilian recreational quadcopter LANDED with propellers stopped and a custom dual-stick handheld controller next to it. Both products completely visible.
Drone: approximately 18 cm wide, four clearly distinct circular propeller guards, exactly four rotors with realistic blades, lightweight open guard construction, a tiny warm-ivory and orange 3D-printed science-fiction explorer canopy around a compact central airframe. Two small non-camera blue status LEDs form a friendly face; no camera lens, no weapon, no robotic arms, no cargo or spray. Visible lightweight frame members, believable landing feet and small battery underneath. Attractive little exploratory companion, not a huge heavy humanoid robot. Clearly a quadcopter, not a spaceship.
Controller: approximately 24 cm wide, charcoal and ivory simple 3D-printed housing, two analog sticks, small monochrome status display reading "SCOUT-01" and "DISARMED", a guarded arm switch and a few clear function buttons. Its enclosure has realistic space for the NU-54V DK board, but don't depict or invent the PCB. This board belongs in the controller; the drone has its own dedicated flight controller hidden inside. No implication that one Bluetooth development board directly powers motors.
In background a soft-focus simple landing target marked with a circle. No persons or animals near the drone.
Editorial product photography, sophisticated but buildable hobby prototype aesthetics, subtle layer lines and fasteners, shallow depth of field, no holograms, no flying effects.
Title at upper left "SCOUT-01", subtitle "BUILD THE CONTROLLER. SHAPE THE EXPLORER." Small caption bottom left "CONCEPT / NOT FLIGHT-TESTED".
Do not add additional slogans or labels anywhere. Not a proven structural design; illustrate the intended appearance only.
```
