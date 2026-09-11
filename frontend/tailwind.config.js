/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        nord: {
          dark: "#09090b",
          surface: "#121215",
          card: "#18181b",
          "card-hover": "#222226",
          cyan: "#00d4b2",
          "cyan-hover": "#00bfa0",
          amber: "#ffb224",
          rose: "#f43f5e",
          emerald: "#10b981",
        }
      },
      fontFamily: {
        sans: ["Geist Sans", "Satoshi", "Inter", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      }
    },
  },
  plugins: [],
}
