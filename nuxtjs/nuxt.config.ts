export default defineNuxtConfig({
  compatibilityDate: '2024-11-01',
  devtools: { enabled: true },
  modules: ['@pinia/nuxt'],
  pages: true,
  typescript: {
    strict: true,
    typeCheck: true,
  },
  css: ['~/assets/css/global.css'], // TODO this is probably not the best way to do this
})
