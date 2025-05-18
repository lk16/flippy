export default defineNuxtConfig({
  compatibilityDate: '2024-11-01',
  devtools: { enabled: true },
  modules: ['@pinia/nuxt'],
  pages: true,
  typescript: {
    strict: true,
    typeCheck: true,
  },
})
