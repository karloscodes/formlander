# Form Examples

This folder contains example HTML files showing how to integrate Formlander with your website.

## Custom Success/Error Redirects

Formlander supports custom redirects after form submission using special hidden input fields:

- `_success_url` - Redirect here after successful submission
- `_error_url` - Redirect here if submission fails (optional)

**These fields are NOT saved in your submission data** - they are only used for controlling redirects.

## Files

### `simple-form.html`
Basic contact form example with custom redirects. Shows how to:
- Set up a native HTML form (no JavaScript required)
- Use `_success_url` and `_error_url` hidden fields
- Configure form action with slug and token

### `thank-you.html`
Example success page with animated checkmark. Use this as your `_success_url` destination.

### `error.html`
Example error page. Use this as your `_error_url` destination.

## Usage

1. **Create a form** in Formlander admin panel and note the slug and token
2. **Update** `simple-form.html` with your form slug and token
3. **Customize** the redirect URLs to point to your own success/error pages
4. **Deploy** these files to your website

## Example Form

```html
<form action="http://localhost:8080/forms/contact/submit?token=abc123" method="POST">
    <input type="text" name="name" required>
    <input type="email" name="email" required>
    <textarea name="message" required></textarea>
    
    <!-- Custom redirects -->
    <input type="hidden" name="_success_url" value="/thank-you.html">
    <input type="hidden" name="_error_url" value="/error.html">
    
    <button type="submit">Send</button>
</form>
```

## How It Works

1. User submits form
2. Formlander validates and stores the submission
3. If successful, redirects to `_success_url`
4. If error, redirects to `_error_url` (or returns JSON error if not set)

## JavaScript vs Native Forms

You can use Formlander with:

- **Native HTML forms** (uses redirects) - Works without JavaScript
- **JavaScript/AJAX** (uses JSON response) - More control over UX

Both approaches work with the same endpoint!

## Notes

- The `_success_url` and `_error_url` fields are filtered out before saving submission data
- If you don't provide redirect URLs, the form will return JSON responses instead
- Redirect URLs can be relative (`/thank-you`) or absolute (`https://example.com/thank-you`)
