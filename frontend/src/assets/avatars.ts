export interface AvatarPreset {
  id: string;
  name: string;
  dataUrl: string;
}

export const PRESET_AVATARS: AvatarPreset[] = [
  {
    id: "grass",
    name: "Дёрн",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2385552B"/><rect width="32" height="12" rx="4" fill="%235B8731"/><path d="M4 12 L8 16 L12 12 L16 17 L20 12 L24 16 L28 12 L32 12 L32 8 L0 8 L0 12 Z" fill="%235B8731"/></svg>`,
  },
  {
    id: "diamond",
    name: "Алмаз",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%231E293B"/><polygon points="16,4 28,14 16,28 4,14" fill="%2300D4B2"/><polygon points="16,8 24,14 16,24 8,14" fill="%2338BDF8"/><polygon points="16,10 20,14 16,20 12,14" fill="%23E0F2FE"/></svg>`,
  },
  {
    id: "creeper",
    name: "Крипер",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2322C55E"/><rect x="6" y="8" width="6" height="6" fill="%230F172A"/><rect x="20" y="8" width="6" height="6" fill="%230F172A"/><rect x="12" y="14" width="8" height="6" fill="%230F172A"/><rect x="9" y="17" width="14" height="9" fill="%230F172A"/><rect x="12" y="23" width="8" height="5" fill="%2322C55E"/></svg>`,
  },
  {
    id: "sword",
    name: "Меч",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2318181B"/><line x1="8" y1="24" x2="24" y2="8" stroke="%2338BDF8" stroke-width="4" stroke-linecap="round"/><line x1="12" y1="20" x2="20" y2="12" stroke="%23E0F2FE" stroke-width="2"/><line x1="7" y1="21" x2="11" y2="25" stroke="%23EAB308" stroke-width="3"/><circle cx="6" cy="26" r="2" fill="%23713F12"/></svg>`,
  },
  {
    id: "pickaxe",
    name: "Кирка",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2318181B"/><path d="M8 8 C14 4, 22 4, 26 10 C22 14, 18 10, 18 10" fill="none" stroke="%23A855F7" stroke-width="4" stroke-linecap="round"/><line x1="8" y1="24" x2="20" y2="12" stroke="%23854D0E" stroke-width="3" stroke-linecap="round"/></svg>`,
  },
  {
    id: "heart",
    name: "Хардкор",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2318181B"/><path d="M16 26 C16 26, 6 18, 6 11 C6 7, 9 5, 12 5 C14 5, 15.5 6, 16 7 C16.5 6, 18 5, 20 5 C23 5, 26 7, 26 11 C26 18, 16 26, 16 26 Z" fill="%23EF4444"/><circle cx="11" cy="9" r="1.5" fill="%23FECACA"/></svg>`,
  },
  {
    id: "furnace",
    name: "Печь",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2352525B"/><rect x="6" y="6" width="20" height="8" rx="2" fill="%2327272A"/><rect x="8" y="18" width="16" height="8" rx="2" fill="%2318181B"/><polygon points="16,19 13,24 19,24" fill="%23F97316"/><polygon points="16,21 14,24 18,24" fill="%23FACC15"/></svg>`,
  },
  {
    id: "potion",
    name: "Зелье",
    dataUrl: `data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="4" fill="%2318181B"/><rect x="13" y="4" width="6" height="3" fill="%23A1A1AA"/><rect x="14" y="7" width="4" height="3" fill="%2371717A"/><path d="M12 10 L20 10 L24 24 C24 27, 21 28, 16 28 C11 28, 8 27, 8 24 Z" fill="%2306B6D4"/><circle cx="13" cy="20" r="1.5" fill="%23CFFAFE"/><circle cx="18" cy="18" r="1" fill="%23CFFAFE"/></svg>`,
  },
];

export function getRandomAvatar(): AvatarPreset {
  const idx = Math.floor(Math.random() * PRESET_AVATARS.length);
  return PRESET_AVATARS[idx];
}
