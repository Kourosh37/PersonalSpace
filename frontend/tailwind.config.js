/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        ink: '#101418',
        mist: '#eff5f5',
        pine: '#2d5f5d',
        sea: '#6bb7b2',
      },
    },
  },
  plugins: [],
};