/**
 * Formlander SDK
 * Auto-hooks to forms and provides retry logic with exponential backoff.
 * Include this script and it will automatically enhance Formlander forms.
 *
 * Usage:
 *   <script src="https://your-formlander.com/assets/formlander.js"></script>
 *
 * Forms are auto-detected by:
 *   - action URL matching /forms/{slug}/submit pattern
 *   - data-formlander attribute on the form element
 */
(function () {
  'use strict';

  var MAX_RETRIES = 3;
  var RETRY_DELAYS = [1000, 2000, 4000];

  function isFormlanderForm(form) {
    if (form.hasAttribute('data-formlander')) return true;
    var action = form.getAttribute('action') || '';
    return /\/forms\/[^/]+\/submit/.test(action);
  }

  function sleep(ms) {
    return new Promise(function (resolve) {
      setTimeout(resolve, ms);
    });
  }

  function submitWithRetry(url, formData, attempt) {
    attempt = attempt || 0;

    return fetch(url, {
      method: 'POST',
      body: formData
    })
      .then(function (response) {
        if (response.status === 503 && attempt < MAX_RETRIES) {
          var retryAfter = response.headers.get('Retry-After');
          var delay = retryAfter ? parseInt(retryAfter, 10) * 1000 : RETRY_DELAYS[attempt];
          return sleep(delay).then(function () {
            return submitWithRetry(url, formData, attempt + 1);
          });
        }
        return response;
      })
      .catch(function (err) {
        if (attempt < MAX_RETRIES) {
          return sleep(RETRY_DELAYS[attempt]).then(function () {
            return submitWithRetry(url, formData, attempt + 1);
          });
        }
        throw err;
      });
  }

  function setFormDisabled(form, disabled) {
    var elements = form.elements;
    for (var i = 0; i < elements.length; i++) {
      elements[i].disabled = disabled;
    }
  }

  function getSubmitButton(form) {
    return form.querySelector('[type="submit"], button:not([type="button"]):not([type="reset"])');
  }

  function showMessage(form, type, text) {
    var msg = form.querySelector('[data-formlander-msg]');
    if (!msg) {
      msg = document.createElement('div');
      msg.setAttribute('data-formlander-msg', '');
      form.appendChild(msg);
    }
    var base = 'mt-4 p-3 rounded-md text-sm';
    var styles = type === 'success'
      ? 'bg-green-50 text-green-800 border border-green-200'
      : 'bg-red-50 text-red-800 border border-red-200';
    msg.className = base + ' ' + styles;
    msg.textContent = text;
  }

  function clearMessage(form) {
    var msg = form.querySelector('[data-formlander-msg]');
    if (msg) {
      msg.parentNode.removeChild(msg);
    }
  }

  function hookForm(form) {
    form.addEventListener('submit', function (e) {
      e.preventDefault();

      var submitBtn = getSubmitButton(form);
      var originalText = submitBtn ? submitBtn.textContent : '';

      clearMessage(form);
      setFormDisabled(form, true);
      if (submitBtn) submitBtn.textContent = 'Sending...';

      var formData = new FormData(form);
      var successUrl = formData.get('_success_url');
      var errorUrl = formData.get('_error_url');

      submitWithRetry(form.action, formData)
        .then(function (response) {
          return response.json().catch(function () {
            return {};
          }).then(function (data) {
            return { response: response, data: data };
          });
        })
        .then(function (result) {
          var response = result.response;
          var data = result.data;

          if (response.ok && data.ok) {
            if (successUrl) {
              window.location.href = successUrl;
            } else {
              form.reset();
              showMessage(form, 'success', 'Form submitted successfully!');
            }
          } else {
            if (errorUrl) {
              window.location.href = errorUrl;
            } else {
              showMessage(form, 'error', data.error || 'Submission failed. Please try again.');
            }
          }
        })
        .catch(function () {
          // Final fallback: submit normally (let browser handle it)
          setFormDisabled(form, false);
          form.submit();
        })
        .finally(function () {
          setFormDisabled(form, false);
          if (submitBtn) submitBtn.textContent = originalText;
        });
    });
  }

  function init() {
    var forms = document.querySelectorAll('form');
    for (var i = 0; i < forms.length; i++) {
      if (isFormlanderForm(forms[i])) {
        hookForm(forms[i]);
      }
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
