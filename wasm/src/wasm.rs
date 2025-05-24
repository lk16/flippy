/// Logs an error message to the console
pub fn console_error(message: &str) {
    web_sys::console::error_1(&message.into());
}

/// Logs a message to the console
pub fn console_log(message: &str) {
    web_sys::console::log_1(&message.into());
}

/// Returns the current time in seconds as a f64
pub fn current_time() -> f64 {
    js_sys::Date::now() / 1000.0
}
