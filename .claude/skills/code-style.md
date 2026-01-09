# Code Style

## Philosophy

Write beautiful, expressive code that is simple and pragmatic. Prefer clarity over cleverness.

## Principles

### 1. Express Intent Clearly

```go
// Good - intent is clear
func (f *Form) IsActive() bool {
    return f.Status == StatusActive && !f.DeletedAt.Valid
}

// Avoid - requires mental parsing
func (f *Form) IsActive() bool {
    return f.Status == StatusActive && f.DeletedAt.Valid == false
}
```

### 2. Keep Functions Short and Focused

Each function should do one thing well. If you need comments to explain sections, extract them.

```go
// Good - each step is a clear function
func ProcessSubmission(ctx context.Context, form *Form, data map[string]any) error {
    if err := validateSubmission(form, data); err != nil {
        return err
    }
    submission := createSubmission(form, data)
    if err := saveSubmission(ctx, submission); err != nil {
        return err
    }
    return dispatchEvents(ctx, submission)
}

// Avoid - monolithic functions with inline comments
func ProcessSubmission(ctx context.Context, form *Form, data map[string]any) error {
    // Validate...
    // 50 lines of validation

    // Create...
    // 30 lines of creation

    // Save...
    // 20 lines of saving

    // Events...
    // 40 lines of event dispatch
}
```

### 3. Name Things Well

Names should reveal intent. Avoid abbreviations unless universally understood.

```go
// Good
userCount := len(users)
isExpired := token.ExpiresAt.Before(time.Now())
canDelete := user.Role == RoleAdmin && !form.HasSubmissions()

// Avoid
uc := len(users)
exp := token.ExpiresAt.Before(time.Now())
cd := user.Role == RoleAdmin && !form.HasSubmissions()
```

### 4. Embrace Early Returns

Reduce nesting by handling edge cases first.

```go
// Good - flat and readable
func GetUser(db *gorm.DB, id uint) (*User, error) {
    if id == 0 {
        return nil, ErrInvalidID
    }

    var user User
    if err := db.First(&user, id).Error; err != nil {
        return nil, err
    }

    return &user, nil
}

// Avoid - unnecessary nesting
func GetUser(db *gorm.DB, id uint) (*User, error) {
    if id != 0 {
        var user User
        if err := db.First(&user, id).Error; err == nil {
            return &user, nil
        } else {
            return nil, err
        }
    }
    return nil, ErrInvalidID
}
```

### 5. Avoid Premature Abstraction

Write concrete code first. Abstract only when you see real duplication.

```go
// Good - solve the actual problem
func SendWelcomeEmail(user *User) error {
    return mailer.Send(user.Email, "Welcome!", welcomeTemplate(user))
}

func SendPasswordReset(user *User, token string) error {
    return mailer.Send(user.Email, "Reset Password", resetTemplate(user, token))
}

// Avoid - over-engineered from the start
type EmailStrategy interface {
    Template() string
    Subject() string
    Recipients() []string
}

type EmailSender struct {
    strategy EmailStrategy
    mailer   Mailer
}
```

### 6. Let the Code Breathe

Use whitespace to group related statements. One blank line between logical sections.

```go
func CreateForm(logger *slog.Logger, db *gorm.DB, params FormParams) (*Form, error) {
    if err := params.Validate(); err != nil {
        return nil, err
    }

    form := &Form{
        Name:   params.Name,
        Slug:   slugify(params.Name),
        Status: StatusActive,
    }

    if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
        return tx.Create(form).Error
    }); err != nil {
        return nil, fmt.Errorf("create form: %w", err)
    }

    return form, nil
}
```

### 7. Error Messages Should Help

Include context that helps debugging.

```go
// Good - actionable error
return fmt.Errorf("create form %q: %w", params.Name, err)

// Avoid - unhelpful
return fmt.Errorf("failed: %w", err)
```

## Anti-Patterns to Avoid

- **Clever one-liners** that require explanation
- **Deep nesting** beyond 2-3 levels
- **Generic names** like `data`, `info`, `item`, `result`
- **Boolean parameters** that obscure meaning at call sites
- **Comments explaining what** instead of why
- **Dead code** left "just in case"

## Remember

> "Any fool can write code that a computer can understand. Good programmers write code that humans can understand." — Martin Fowler
