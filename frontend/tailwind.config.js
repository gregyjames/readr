module.exports = {
  darkMode: 'class',
  purge: [
    './src/**/*.html',
    './src/**/*.vue',
    './src/**/*.jsx',
  ],
  theme: {},
  variants: {},
  plugins: [
    require('@tailwindcss/typography')
  ],
}